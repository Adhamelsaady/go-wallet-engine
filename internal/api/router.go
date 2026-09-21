package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)
func NewRouter(a *AccountHandler , t *TransferHandler , admin *AdminHandler , AdminApiKey string) *chi.Mux {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Get("/health" , HandleHealth)
	router.Route("/accounts" , func (r chi.Router)  {
		r.Post("/" , a.CreateAccount)
		r.Get("/{id}/balance" , a.GetBalance)
		r.Get("/{id}/entries" , a.GetEntries)
	})
	router.Route("/transfers" , func(r chi.Router) {
		r.Post("/" , t.CreateTransfer)
	})
	router.Route("/admin" , func(r chi.Router) {
		r.Use(AdminAuthMiddleware(AdminApiKey))
		r.Get("/transactions/{id}" , admin.GetTransactionDetails)
	})
	return router
}