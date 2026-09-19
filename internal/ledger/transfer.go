package ledger

import "github.com/google/uuid"
import "time"

type TransferParams struct {
	FromAccountId uuid.UUID 
	ToAccountId uuid.UUID 
	Amount int64 
	Currency string
	Description string
	IdempotencyKey string
	RequestFingerPrint string
}

type TransferResponse struct {
	TransactionId uuid.UUID `json:"transaction_id"`
	FromAccountId uuid.UUID `json:"from_account_id"`
	ToAccountId uuid.UUID `json:"to_account_id"`
	Amount int64 `json:"amount"`
	Currency string `json:"currency"`
	Description string `json:"description"`
	Status string `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}