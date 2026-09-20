package storage_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/assert"
	"github.com/adhamelsaady/digital-wallet/internal/db"
	"github.com/adhamelsaady/digital-wallet/internal/ledger"
	"github.com/adhamelsaady/digital-wallet/internal/storage"
)

// setupTestPool connects to the live test database using DATABASE_URL.
func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://wallet:wallet_secret@localhost:5433/digital_wallet?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.New(ctx, dsn)
	require.NoError(t, err, "failed to connect to test db")

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// truncateTables clears all tables between test runs to ensure state isolation.
func truncateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), "TRUNCATE accounts, transactions, ledger_entries CASCADE;")
	require.NoError(t, err, "failed to truncate tables")
}

// seedAccount creates an account and seeds an initial CREDIT ledger entry.
func seedAccount(t *testing.T, pool *pgxpool.Pool, repo *storage.AccountRepository, currency string, initialBalance int64) *ledger.Account {
	t.Helper()
	ctx := context.Background()
	acc, err := ledger.NewAccount(uuid.New(), currency, "AVAILABLE")
	require.NoError(t, err)

	err = repo.CreateAccount(ctx, acc)
	require.NoError(t, err)

	if initialBalance > 0 {
		txID := uuid.New()
		_, err := pool.Exec(ctx,
			`INSERT INTO transactions (id, reference_type, description, status) VALUES ($1, 'DEPOSIT', 'Initial seed', 'COMPLETED')`,
			txID,
		)
		require.NoError(t, err)

		_, err = pool.Exec(ctx,
			`INSERT INTO ledger_entries (id, transaction_id, account_id, amount, entry_type) VALUES ($1, $2, $3, $4, 'CREDIT')`,
			uuid.New(), txID, acc.ID, initialBalance,
		)
		require.NoError(t, err)
	}
	return acc
}


func TestTransferRepository_ExecuteTransfer_InsufficientFunds(t *testing.T) {
	pool := setupTestPool(t)
	truncateTables(t, pool)

	accRepo := storage.NewAccountRepository(pool)
	transferRepo := storage.NewTransferRepository(pool)

	// Account A has only $30.00 (3,000 cents), Account B has $0.00
	accA := seedAccount(t, pool, accRepo, "USD", 3000)
	accB := seedAccount(t, pool, accRepo, "USD", 0)

	ctx := context.Background()
	params := ledger.TransferParams{
		FromAccountId: accA.ID,
		ToAccountId: accB.ID,
		Amount: 5000, // Wants $50.00 (more than available)
		Description: "Overdraft attempt",
	}

	resp, err := transferRepo.ExecuteTransfer(ctx, params)

	// Assertions on error
	require.Error(t, err)
	assert.ErrorIs(t, err, ledger.ErrorInsufficientFunds)
	assert.Nil(t, resp)

	// Verify balances did NOT change
	balA, err := accRepo.CalculateBalance(ctx, accA.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), balA, "Account A balance should remain unchanged")

	balB, err := accRepo.CalculateBalance(ctx, accB.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), balB, "Account B balance should remain 0")

	// Verify no orphan transaction was created (excluding initial seed)
	var txCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM transactions WHERE reference_type = 'TRANSFER'").Scan(&txCount)
	require.NoError(t, err)
	assert.Equal(t, 0, txCount, "no transfer transaction should be saved")

	// Verify no transfer ledger entries were written (excluding initial seed)
	var entryCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM ledger_entries WHERE amount = 5000").Scan(&entryCount)
	require.NoError(t, err)
	assert.Equal(t, 0, entryCount, "no ledger entries should be saved")
}
func TestTransferRepository_ExecuteTransfer_Success(t *testing.T) {
	pool := setupTestPool(t)
	truncateTables(t, pool)

	accRepo := storage.NewAccountRepository(pool)
	transferRepo := storage.NewTransferRepository(pool)

	// Account A has $100.00 (10,000 cents), Account B has $0.00
	accA := seedAccount(t, pool, accRepo, "USD", 10000)
	accB := seedAccount(t, pool, accRepo, "USD", 0)

	ctx := context.Background()
	params := ledger.TransferParams{
		FromAccountId: accA.ID,
		ToAccountId: accB.ID,
		Amount: 4000, // Transfer $40.00
		Description: "Payment for services",
	}

	resp, err := transferRepo.ExecuteTransfer(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify response fields
	assert.Equal(t, accA.ID, resp.FromAccountId)
	assert.Equal(t, accB.ID, resp.ToAccountId)
	assert.Equal(t, int64(4000), resp.Amount)
	assert.Equal(t, "USD", resp.Currency)
	assert.Equal(t, "COMPLETED", resp.Status)
	assert.Equal(t, "Payment for services", resp.Description)

	// Verify updated dynamic balances in DB
	balA, err := accRepo.CalculateBalance(ctx, accA.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(6000), balA, "Account A balance should decrease from 10,000 to 6,000")

	balB, err := accRepo.CalculateBalance(ctx, accB.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(4000), balB, "Account B balance should increase from 0 to 4,000")

	// Verify DB state: exactly 1 transaction with COMPLETED status
	var txStatus string
	err = pool.QueryRow(ctx, "SELECT status FROM transactions WHERE id = $1", resp.TransactionId).Scan(&txStatus)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETED", txStatus)

	// Verify DB state: exactly 2 ledger entries for this transaction
	rows, err := pool.Query(ctx, "SELECT account_id, amount, entry_type FROM ledger_entries WHERE transaction_id = $1", resp.TransactionId)
	require.NoError(t, err)
	defer rows.Close()

	var debitCount, creditCount int
	for rows.Next() {
		var accountID uuid.UUID
		var amount int64
		var entryType string
		err := rows.Scan(&accountID, &amount, &entryType)
		require.NoError(t, err)

		assert.Equal(t, int64(4000), amount)
		if entryType == "DEBIT" {
			assert.Equal(t, accA.ID, accountID)
			debitCount++
		} else if entryType == "CREDIT" {
			assert.Equal(t, accB.ID, accountID)
			creditCount++
		}
	}
	assert.Equal(t, 1, debitCount, "should have exactly 1 debit entry")
	assert.Equal(t, 1, creditCount, "should have exactly 1 credit entry")
}
