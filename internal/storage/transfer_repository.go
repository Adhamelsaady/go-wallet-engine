package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/adhamelsaady/digital-wallet/internal/ledger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	referenceTransfer = "TRANSFER"
	statusCompleted = "COMPLETED"
	entryDebit = "DEBIT"
	entryCredit = "CREDIT"
)
type TransferRepository struct {
	pool *pgxpool.Pool
}
type storedTransaction struct {
	TransactionID uuid.UUID
	FromAccountID uuid.UUID
	ToAccountID uuid.UUID
	Amount int64
	Currency string
	Description string
	Status string
	RequestFingerprint string
	CreatedAt time.Time
}
func (s *storedTransaction) toTransferResponse() *ledger.TransferResponse {
	return &ledger.TransferResponse{
		TransactionId: s.TransactionID,
		FromAccountId: s.FromAccountID,
		ToAccountId: s.ToAccountID,
		Amount: s.Amount,
		Currency: s.Currency,
		Description: s.Description,
		Status: s.Status,
		CreatedAt: s.CreatedAt,
	}
}


func NewTransferRepository(pool *pgxpool.Pool) *TransferRepository {
	return &TransferRepository{pool: pool}
}

func (r *TransferRepository) ExecuteTransfer(ctx context.Context, transferParams ledger.TransferParams) (*ledger.TransferResponse, error) {
	
	if transferParams.IdempotencyKey != "" {
		stored, err := r.findByIdempotencyKey(ctx , transferParams.IdempotencyKey)
		if err != nil {
			return nil , fmt.Errorf("check idempotency key: %w", err)
		} 
		if stored != nil {
			if stored.RequestFingerprint != transferParams.RequestFingerPrint {
				return nil , ledger.ErrorIdempotencyKeyDuplicate
			} 
			return stored.toTransferResponse(), nil
		}
	}
	
	// begin transaction
	transaction , err := r.pool.Begin(ctx)
	if err != nil {
		return nil , fmt.Errorf("begin transaction: %w", err)
	}
	// defer rollback
	defer transaction.Rollback(ctx)
	
	from , to , err := r.lockAccountsInOrder(ctx , transaction , transferParams.FromAccountId , transferParams.ToAccountId)
	if err != nil {
		return nil, err
	}

	err = validateTransfer(from , to, transferParams.Amount)
	if err != nil {
		return nil, fmt.Errorf("validate transfer: %w", err)
	}
	balance , err := calculateBalanceTx(ctx, transaction, from.ID)
	if err != nil {
		return nil, fmt.Errorf("calculate sender balance: %w", err)
	}
	if balance < transferParams.Amount {
		return nil, ledger.ErrorInsufficientFunds
	}
	transactionId := uuid.New()
	curTime := time.Now().UTC()
	if err := createTransactionByIdempotencyKey(ctx , transaction , transactionId , transferParams.Description , transferParams.IdempotencyKey , transferParams.RequestFingerPrint , curTime); err != nil {
		return nil, err
	}

	if err != nil{
		var pgxError *pgconn.PgError
		if errors.As(err, &pgxError) && pgxError.Code == "23505" {
			existing , fetchErr := r.findByIdempotencyKey(ctx , transferParams.IdempotencyKey)
			if fetchErr != nil || existing == nil {
				return nil , fmt.Errorf("concurrent idempotency collision recovery failed : %w" , err)
			}
			return existing.toTransferResponse() , nil // return the existing one
		}
		return nil , err
	}

	if err := createLedgerEntry(ctx , transaction , transactionId, transferParams.FromAccountId , transferParams.Amount , entryDebit , curTime); err != nil {
		return nil, err
	}
	if err := createLedgerEntry(ctx , transaction , transactionId, transferParams.ToAccountId , transferParams.Amount , entryCredit , curTime); err != nil {
		return nil, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}
	return &ledger.TransferResponse{
		TransactionId: transactionId,
		FromAccountId: from.ID,
		ToAccountId: to.ID,
		Amount: transferParams.Amount,
		Currency: from.Currency,
		Description: transferParams.Description,
		Status: statusCompleted,
		CreatedAt: curTime,
	},nil

}
func getAccountById (ctx context.Context , transaction pgx.Tx , id uuid.UUID) (*ledger.Account , error) {
	query := `SELECT id, owner_id, currency, type, created_at, updated_at FROM accounts WHERE id = $1 FOR UPDATE`
	var account ledger.Account
	err := transaction.QueryRow(ctx , query , id).Scan(
		&account.ID,
		&account.OwnerID,
		&account.Currency,
		&account.Type,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ledger.ErrorAccountNotFound
		}

		return nil, fmt.Errorf(
			"get account %s: %w",
			id,
			err,
		) 
	}
	return &account, nil
}
func validateTransfer(from *ledger.Account,to *ledger.Account,amount int64) error {
	if from.ID == to.ID {
		return ledger.ErrorSelfTransfer
	}
	if amount <= 0 {
		return ledger.ErrorInvalidAmount
	}
	if from.Currency != to.Currency {
		return ledger.ErrorCurrencyMismatch
	}
	return nil
}

func calculateBalanceTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) (int64, error) {
	const query = `
		SELECT COALESCE(
			SUM(
				CASE
					WHEN entry_type = 'CREDIT' THEN amount
					WHEN entry_type = 'DEBIT'  THEN -amount
					ELSE 0
				END
			),
			0
		)
		FROM ledger_entries	WHERE account_id = $1
	`
	var balance int64
	err := tx.QueryRow(ctx,query,accountID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("calculate balance for %s: %w",accountID,err)
	}
	return balance, nil
}

func createLedgerEntry(ctx context.Context, tx pgx.Tx, transactionID uuid.UUID, accountID uuid.UUID, amount int64, entryType string, createdAt time.Time) error {
	const query = `INSERT INTO ledger_entries(id, transaction_id, account_id, amount, entry_type, created_at) 
				   VALUES ($1,$2,$3,$4,$5,$6)`
	_, err := tx.Exec(ctx, query,uuid.New(), transactionID, accountID, amount, entryType, createdAt)
	if err != nil {
		return fmt.Errorf("create %s ledger entry: %w",	entryType, err)
	}
	return nil
}

func (r *TransferRepository) findByIdempotencyKey (ctx context.Context , key string) (*storedTransaction , error) {
	const query = `SELECT t.id, debit_entry.account_id AS from_account_id, credit_entry.account_id AS to_account_id, debit_entry.amount, a.currency, COALESCE(t.description, ''), t.status, COALESCE(t.request_fingerprint, ''), t.created_at
				   FROM transactions t JOIN ledger_entries debit_entry 
				   ON debit_entry.transaction_id = t.id AND debit_entry.entry_type = 'DEBIT'
				   JOIN ledger_entries credit_entry ON credit_entry.transaction_id = t.id AND credit_entry.entry_type = 'CREDIT'
				   JOIN accounts a ON a.id = debit_entry.account_id
				   WHERE t.idempotency_key = $1`
	var stored storedTransaction
	err := r.pool.QueryRow(ctx,query,key).Scan(
		&stored.TransactionID,
		&stored.FromAccountID,
		&stored.ToAccountID,
		&stored.Amount,
		&stored.Currency,
		&stored.Description,
		&stored.Status,
		&stored.RequestFingerprint,
		&stored.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ledger.ErrorIdempotencyKeyDuplicate
		}
		return nil, fmt.Errorf("find by idempotency key: %w", err)
	}
	return &stored, nil
}

func (r *TransferRepository) lockAccountsInOrder (ctx context.Context , tx pgx.Tx , fromId uuid.UUID , toId uuid.UUID) (*ledger.Account , *ledger.Account , error) {
	firstId , secondId := fromId , toId
	if firstId.String() > secondId.String() {
		firstId, secondId = toId, fromId
	}
	firstAcc , err := getAccountById(ctx , tx , firstId)
	if err != nil {
		return nil , nil , err
	}
	secondAcc , err := getAccountById(ctx , tx , secondId)
	if err != nil {
		return nil , nil , err
	}
	if fromId == firstAcc.ID {
		return firstAcc , secondAcc , nil
	}
	return secondAcc , firstAcc , nil
}

func  createTransactionByIdempotencyKey(ctx context.Context , tx pgx.Tx , transctionId uuid.UUID , description string , idempotencyKey string , requestFingerprint string , createdAt time.Time ) error {
	const query = `INSERT INTO transactions(id , reference_type , description , status , idempotency_key , request_fingerprint , created_at) 
					values ($1,$2,$3,$4,$5,$6,$7)`
	_, err := tx.Exec(ctx , query , transctionId, referenceTransfer , description , statusCompleted , nullableString(idempotencyKey) , nullableString(requestFingerprint) , createdAt)
	if err != nil {
		return fmt.Errorf("create transaction by idempotency key: %w",err)
	}
	return nil
}

func nullableString (s string) *string {
	if s == "" {
		return nil
	}
	return &s
}