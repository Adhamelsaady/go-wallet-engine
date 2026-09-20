package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAccountStore struct{}

func (m *mockAccountStore) GetAccountById(_ context.Context, _ uuid.UUID) (*Account, error) {
	return &Account{}, nil
}
func (m *mockAccountStore) CalculateBalance(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}

type mockTransferStore struct {
	called bool
}

func (m *mockTransferStore) ExecuteTransfer(_ context.Context, params TransferParams) (*TransferResponse, error) {
	m.called = true
	return &TransferResponse{
		TransactionId: uuid.New(),
		FromAccountId: params.FromAccountId,
		ToAccountId: params.ToAccountId,
		Amount: params.Amount,
		Currency: "USD",
		Status: "COMPLETED",
	}, nil
}

func TestCreateTransfer_Validation(t *testing.T) {
	tests := []struct {
		name string
		params TransferParams
		wantErr error
		wantCall bool
	}{
		{
			name: "valid transfer passes domain checks and calls store",
			params: TransferParams{
				FromAccountId: uuid.New(),
				ToAccountId: uuid.New(),
				Amount: 1000,
			},
			wantErr: nil,
			wantCall: true,
		},
		{
			name: "zero amount is rejected before reaching the store",
			params: TransferParams{
				FromAccountId: uuid.New(),
				ToAccountId: uuid.New(),
				Amount: 0,
			},
			wantErr: ErrorInvalidAmount,
			wantCall: false,
		},
		{
			name: "negative amount is rejected before reaching the store",
			params: TransferParams{
				FromAccountId: uuid.New(),
				ToAccountId: uuid.New(),
				Amount: -500,
			},
			wantErr: ErrorInvalidAmount,
			wantCall: false,
		},
		{
			name: "self-transfer is rejected before reaching the store",
			params: func() TransferParams {
				id := uuid.New()
				return TransferParams{
					FromAccountId: id,
					ToAccountId: id,
					Amount: 1000,
				}
			}(),
			wantErr: ErrorSelfTransfer,
			wantCall: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockTransfer := &mockTransferStore{}
			svc := NewService(&mockAccountStore{}, mockTransfer)
			result, err := svc.CreateTransfer(context.Background(), tc.params)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
			assert.Equal(t, tc.wantCall, mockTransfer.called)
		})
	}
}


func TestLockOrder(t *testing.T) {
	id1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	id2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	tests := []struct {
		name string
		idA uuid.UUID
		idB uuid.UUID
		wantFirst uuid.UUID
		wantSecond uuid.UUID
	}{
		{
			name: "already in order (idA < idB)",
			idA: id1,
			idB: id2,
			wantFirst: id1,
			wantSecond: id2,
		},
		{
			name: "reverse order (idA > idB) gets flipped",
			idA: id2,
			idB: id1,
			wantFirst: id1,
			wantSecond: id2,
		},
		{
			name: "identical IDs return same order",
			idA: id1,
			idB: id1,
			wantFirst: id1,
			wantSecond: id1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			first, second := LockOrder(tc.idA, tc.idB)
			assert.Equal(t, tc.wantFirst, first)
			assert.Equal(t, tc.wantSecond, second)
		})
	}
}

