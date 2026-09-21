package main

import (
	"context"
	"log"
	"net/http"

	"github.com/adhamelsaady/digital-wallet/internal/api"
	"github.com/adhamelsaady/digital-wallet/internal/config"
	"github.com/adhamelsaady/digital-wallet/internal/db"
	"github.com/adhamelsaady/digital-wallet/internal/storage"
	"github.com/adhamelsaady/digital-wallet/internal/ledger"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	ctx := context.Background()
	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db error: %v", err)
	}
	defer pool.Close()
	log.Println(" database connection established")

	accountRepository := storage.NewAccountRepository(pool)
	transferRepository := storage.NewTransferRepository(pool)
	ledgerService := ledger.NewService(accountRepository , transferRepository)
	accountHandler := api.NewAccountHandler(accountRepository , ledgerService)
	transferHandler := api.NewTransferHandler(ledgerService)
	adminHandler := api.NewAdminHandler(ledgerService)
	router := api.NewRouter(accountHandler, transferHandler , adminHandler , cfg.AdminApiKey)
	addr := cfg.ServerPort
	if addr != "" && addr[0] != ':' {
		addr = ":" + addr
	}
	log.Printf("listening on port : %s" , addr)
	
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}