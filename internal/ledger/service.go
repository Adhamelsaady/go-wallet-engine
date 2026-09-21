package ledger

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type AccountStore interface {
	GetAccountById(ctx context.Context, accountId uuid.UUID) (*Account, error)
	CalculateBalance(ctx context.Context, accountId uuid.UUID) (int64, error)
}

type TransferStore interface {
	ExecuteTransfer(ctx context.Context, params TransferParams) (*TransferResponse, error)
}

type AdminStore interface {
	GetAccountEntries (ctx context.Context , filter EntryFilter) (*PagedResult , error)
}

type HistoryStore interface {
	GetTransactionHistory(ctx context.Context , id uuid.UUID) (*TransactionDetail, error)
}

type Service struct {
	accountRepository AccountStore
	transferRepository TransferStore
}

func NewService(accountRepository AccountStore, transferRepository TransferStore) *Service {
	return &Service{accountRepository: accountRepository, transferRepository: transferRepository}
}

func (s *Service) GetAccountAndBalance(ctx context.Context, accountId uuid.UUID) (*Account, int64, error) {
	account, err := s.accountRepository.GetAccountById(ctx, accountId)
	if err != nil {
		return nil, 0, err
	}
	balance, err := s.accountRepository.CalculateBalance(ctx, accountId)
	if err != nil {
		return nil, 0, fmt.Errorf("ledger.Service: %w", err)
	}
	return account,balance, nil
}

func (s *Service) CreateTransfer (ctx context.Context, params TransferParams) (*TransferResponse, error) {
	if params.Amount <= 0 {
		return nil , ErrorInvalidAmount
	}
	if params.FromAccountId == params.ToAccountId {
		return nil , ErrorSelfTransfer
	}
	result , err := s.transferRepository.ExecuteTransfer(ctx, params)
	if err != nil {
		return nil , fmt.Errorf("ledger.Service: %w", err)
	}
	return result, nil
}
