package api

import (
	"errors"
	"net/http"

	"github.com/adhamelsaady/digital-wallet/internal/ledger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AdminHandler struct {
	ledgerService *ledger.Service
}

func NewAdminHandler (ledgerService *ledger.Service) *AdminHandler {
	return &AdminHandler{ledgerService: ledgerService}
}

func (adminHandler *AdminHandler)GetTransactionDetails (writer http.ResponseWriter , request *http.Request) {
	idStr := chi.URLParam(request , "id")
	transactionId , err := uuid.Parse(idStr)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid transaction ID")
		return
	}
	detail , err := adminHandler.ledgerService.GetTransactionDetails(request.Context() , transactionId)
	if err != nil {
		if errors.Is(err , ledger.ErrorTransactionNotFound) {
			writeError(writer , http.StatusNotFound , "transaction not found")
			return
		} 
		writeError(writer , http.StatusInternalServerError , "failed to fetch transaction details")
		return
	}
	writeJSON(writer , http.StatusOK , detail)
}








