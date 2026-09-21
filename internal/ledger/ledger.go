package ledger

import (
	"errors"
	"time"
	"github.com/google/uuid"
)

type EntryType string
const (
	EntryTypeCredit EntryType = "CREDIT"
	EntryTypeDebit EntryType = "DEBIT"
)

var ErrorInvalidAmount = errors.New("Invalid amount")
var ErrorInvalidEntryType = errors.New("Invalid entry type")
var ErrorTransactionNotFound = errors.New("transaction not found")

type LedgerEntry struct {
	ID uuid.UUID `json:"id"`
	TransactionId uuid.UUID `json:"transaction_id"`
	AccountId uuid.UUID `json:"account_id"`
	Amount int64 `json:"ammount"`
	EntryType EntryType `json:"entry_type"`
	CreatedAt time.Time `json:"time"` 
}

type EntryFilter struct{
	AccountId uuid.UUID
	EntryType *EntryType
	Limit int
	Offset int
}

type PagedResult struct {
	Entries []LedgerEntry `json:"entries"`
	Total int `json:"total"`
	Limit int `json:"limit"`
	Offset int `json:"offset"`
}

type TransactionDetail struct {
	Id uuid.UUID `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	RefrenceType string `json:"reference_type"`
	Description string `json:"description"`
	RequestFingerPrint *string `json:"request_fingerprint,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Status string `json:"status"`
	Entries []LedgerEntry `json:"entries"`
}


func CalculateBalance(entries []LedgerEntry) int64 {
	var balance int64 = 0
	for _, entry := range entries {
		if entry.EntryType == EntryTypeCredit {
			balance += entry.Amount
		} else {
			balance -= entry.Amount
		}
	}
	return balance
}