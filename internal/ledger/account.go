package ledger

import (
	"time"
	"strings"
	"errors"
	"github.com/google/uuid"
)

var supportedCurrencies = map[string]bool {
	"USD": true,
	"EGP": true,
	"SAR": true,
	"AED": true,
	"EUR": true,
}

type Account struct {
	ID uuid.UUID `json:"id"`
	OwnerID uuid.UUID `json:"owner_id"`
	Currency string `json:"currency"`
	Type string `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}


var	ErrorInvalidCurrency = errors.New("Currency not supported")
var	ErrorAccountNotFound = errors.New("Account not found")
var	ErrorInsufficientFunds = errors.New("insufficient funds")
var	ErrorInvalidOwner = errors.New("Invalid owner")
var	ErrorInvalidType = errors.New("Invalid account type")
var ErrorAccountDuplicate = errors.New("Account duplicate")
var ErrorSelfTransfer = errors.New("Cannot transfer to the same account")
var ErrorCurrencyMismatch = errors.New("Accounts must have the same currency")
var ErrorIdempotencyKeyDuplicate = errors.New("Idempotency key duplicate")

func NewAccount (ownerId uuid.UUID, currency string , accountType string) (*Account, error) {
	if ownerId == uuid.Nil {
		return nil , ErrorInvalidOwner
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !supportedCurrencies[currency] {
		return nil , ErrorInvalidCurrency
	}
	accountType = strings.ToUpper(strings.TrimSpace(accountType))
	if accountType == "" {
		accountType = "AVAILABLE"
	}
	curTime := time.Now().UTC()
	return &Account{
		ID: uuid.New(),
		OwnerID: ownerId,
		Currency: currency,
		Type: accountType,
		CreatedAt: curTime,
		UpdatedAt: curTime,
	}, nil
}