# Digital Wallet & Double-Entry Ledger Engine

## Overview

This project implements a production-grade digital wallet and double-entry ledger engine built with Go and PostgreSQL. The system adheres strictly to double-entry bookkeeping principles, guaranteeing that account balances are mathematically derived from immutable ledger entries rather than stored as mutable state.

It provides atomic peer-to-peer money transfers with strict concurrency control, deterministic deadlock prevention, and robust idempotency safeguards to ensure financial consistency under high concurrency and network retries.

---

## Architectural Principles & Decisions

- **Double-Entry Accounting:** Every financial mutation is represented by balanced ledger entries (credits and debits). Account balances are calculated dynamically from an immutable append-only audit trail (`COALESCE(SUM(...), 0)`), preventing lost updates, race conditions, and accounting discrepancies.
- **Atomic Transfers & ACID Guarantees:** Peer-to-peer transfers execute within an isolated PostgreSQL transaction (`pgx.Tx`). Funds are debited from the sender and credited to the recipient atomically; if any step fails, the entire transaction rolls back.
- **Deterministic Deadlock Prevention:** Account rows are locked using `SELECT ... FOR UPDATE` in a deterministic lexicographical order (sorted by UUID). This eliminates deadlocks when concurrent, bidirectional transfers occur between the same accounts (e.g., A $\rightarrow$ B and B $\rightarrow$ A simultaneously).
- **End-to-End Idempotency & SHA-256 Fingerprinting:** Transfer endpoints accept an optional `Idempotency-Key` header. Incoming request payloads are hashed using SHA-256 (`request_fingerprint`). If a retry arrives with the same key and identical payload, the original transaction response is returned without re-executing. If a key is reused with a different payload, a `409 Conflict` is returned. A PostgreSQL unique constraint on `idempotency_key` guarantees safety against simultaneous race conditions.
- **Exact Integer Financials:** All monetary amounts are stored as 64-bit integers (`int64` / PostgreSQL `BIGINT`) representing minor units (e.g., cents, piastres: $10.00 = 1000). Floating-point types are strictly forbidden to eliminate rounding errors.
- **No ORM / Explicit SQL:** Database operations use raw parameterized SQL via `pgx/v5` (`pgxpool`). This provides explicit control over query plans, isolation levels, and row locking.
- **Clean Architecture & Consumer-Driven Interfaces:** Interfaces are declared in the domain layer where they are consumed (`ledger.AccountStore`, `ledger.TransferStore`), keeping the core domain completely decoupled from concrete persistence implementations.
- **Domain-Driven Error Mapping:** Low-level database errors (such as unique key violations, deadlock errors, or `pgx.ErrNoRows`) are mapped to sentinel domain errors in the storage layer, preventing infrastructure leakage into API handlers.
- **Explicit Dependency Injection:** Dependencies are constructed and wired explicitly in `cmd/api/main.go` without reflection or global state.

---

## Tech Stack

- **Language:** Go 1.22+
- **Database:** PostgreSQL 16
- **Database Driver & Connection Pool:** `jackc/pgx/v5` (`pgxpool`)
- **HTTP Router & Middleware:** `go-chi/chi/v5`
- **Identifier Format:** `google/uuid` (UUIDv4)
- **Testing:** Standard `testing` package with `stretchr/testify`
- **Containerization:** Docker & Docker Compose

---

## Project Structure

```
.
├── cmd/
│   └── api/
│       └── main.go                 # Composition root & dependency wiring
├── internal/
│   ├── config/                     # Environment variable parsing
│   ├── db/                         # PostgreSQL connection pool lifecycle (pgxpool)
│   ├── ledger/                     # Pure business domain: entities, invariants, service
│   │   ├── account.go              # Account entity, constructor validation & sentinel errors
│   │   ├── account_test.go         # Table-driven unit tests for Account
│   │   ├── ledger.go               # LedgerEntry entity & balance calculation logic
│   │   ├── ledger_test.go          # Unit tests for ledger operations
│   │   ├── transfer.go             # TransferParams & TransferResponse domain types
│   │   ├── transfer_test.go        # Unit tests for CreateTransfer validation
│   │   └── service.go              # Ledger service, AccountStore & TransferStore interfaces
│   ├── storage/                    # Infrastructure layer: raw SQL repository implementations
│   │   ├── account_repository.go   # PostgreSQL account and balance queries
│   │   └── transfer_repository.go  # Atomic pgx.Tx transfers, row locking & idempotency
│   └── api/                        # HTTP transport layer
│       ├── account_handler.go      # REST handlers for accounts & balances
│       ├── transfer_handler.go     # POST /transfers handler with SHA-256 fingerprinting
│       ├── health.go               # Health check handler
│       ├── response.go             # Standardized JSON response helpers
│       └── router.go               # Chi router setup & middleware pipelines
├── migrations/                     # Plain SQL database migrations
│   ├── 000001_create_accounts_table.up.sql
│   ├── 000001_create_accounts_table.down.sql
│   ├── 000002_create_ledger_entries.up.sql
│   ├── 000002_create_ledger_entries.down.sql
│   ├── 000003_add_idempotency.up.sql
│   └── 000003_add_idempotency.down.sql
├── docker-compose.yml              # Local PostgreSQL 16 service
├── .env.example                    # Sample environment configuration
├── go.mod
└── go.sum
```

---

## Database Schema Design

### 1. `accounts`
Stores identity and currency boundaries. Balances are intentionally not stored here.
- `id` (UUID, Primary Key)
- `owner_id` (UUID, Hash Index)
- `currency` (VARCHAR(3), e.g., 'USD', 'EUR')
- `type` (VARCHAR(32), e.g., 'AVAILABLE')
- `created_at`, `updated_at` (TIMESTAMPTZ)
- *Constraint:* `UNIQUE(owner_id, currency)` prevents duplicate accounts for the same currency.

### 2. `transactions`
Represents an idempotent financial business event.
- `id` (UUID, Primary Key)
- `idempotency_key` (VARCHAR(255), UNIQUE, Nullable)
- `request_fingerprint` (VARCHAR(64), Nullable) — SHA-256 checksum of the incoming request body
- `reference_type` (VARCHAR(64), e.g., 'TRANSFER', 'DEPOSIT')
- `description` (TEXT)
- `status` (VARCHAR(32), DEFAULT 'COMPLETED')
- `created_at` (TIMESTAMPTZ)

### 3. `ledger_entries`
Immutable append-only ledger entries associated with a transaction.
- `id` (UUID, Primary Key)
- `transaction_id` (UUID, Foreign Key $\rightarrow$ `transactions.id` ON DELETE CASCADE)
- `account_id` (UUID, Foreign Key $\rightarrow$ `accounts.id` ON DELETE RESTRICT)
- `amount` (BIGINT, CHECK `amount > 0`)
- `entry_type` (VARCHAR(6), CHECK `entry_type IN ('DEBIT', 'CREDIT')`)
- `created_at` (TIMESTAMPTZ)
- *Indexes:* `idx_ledger_entries_account_id`, `idx_ledger_entries_transaction_id`

---

## Getting Started

### 1. Prerequisites
- Go 1.22+
- Docker and Docker Compose

### 2. Environment Configuration
Create a `.env` file in the project root:
```env
PORT=8080
DATABASE_URL=postgres://wallet:wallet_secret@localhost:5433/digital_wallet?sslmode=disable
```

### 3. Start Database
```powershell
docker compose up -d
```

### 4. Run Migrations
Apply migration files in sequence:

**PowerShell (Windows):**
```powershell
Get-Content migrations/000001_create_accounts_table.up.sql | docker exec -i digital_wallet_db psql -U wallet -d digital_wallet
Get-Content migrations/000002_create_ledger_entries.up.sql | docker exec -i digital_wallet_db psql -U wallet -d digital_wallet
Get-Content migrations/000003_add_idempotency.up.sql | docker exec -i digital_wallet_db psql -U wallet -d digital_wallet
```

**Bash / Linux / macOS:**
```bash
docker exec -i digital_wallet_db psql -U wallet -d digital_wallet < migrations/000001_create_accounts_table.up.sql
docker exec -i digital_wallet_db psql -U wallet -d digital_wallet < migrations/000002_create_ledger_entries.up.sql
docker exec -i digital_wallet_db psql -U wallet -d digital_wallet < migrations/000003_add_idempotency.up.sql
```

### 5. Run Tests
Execute all unit tests across domain packages:
```powershell
go test -v ./internal/ledger/...
```

### 6. Start the API Server
```powershell
go run ./cmd/api
```

---

## API Endpoints

| Method | Path | Description | Success Code |
|---|---|---|---|
| `GET` | `/health` | Liveness check | `200 OK` |
| `POST` | `/accounts` | Create a new account | `201 Created` |
| `GET` | `/accounts/{id}/balance` | Fetch account details and current derived balance | `200 OK` |
| `POST` | `/transfers` | Atomically transfer funds between accounts (supports `Idempotency-Key`) | `201 Created` |

---

## API Examples

### Create Account

**Request:**
```powershell
curl -X POST http://localhost:8080/accounts `
  -H "Content-Type: application/json" `
  -d '{"owner_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "currency": "USD", "type": "AVAILABLE"}'
```

**Response (`201 Created`):**
```json
{
  "id": "1e26d609-a543-4845-b3be-9a7756303b4d",
  "owner_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
  "currency": "USD",
  "type": "AVAILABLE",
  "created_at": "2026-09-13T20:06:57.184Z",
  "updated_at": "2026-09-13T20:06:57.184Z"
}
```

---

### Get Account Balance

**Request:**
```powershell
curl -X GET http://localhost:8080/accounts/1e26d609-a543-4845-b3be-9a7756303b4d/balance
```

**Response (`200 OK`):**
```json
{
  "account_id": "1e26d609-a543-4845-b3be-9a7756303b4d",
  "currency": "USD",
  "balance": 6500
}
```

> Note: The balance is returned in minor units (e.g., `6500` = $65.00).

---

### Transfer Funds (with Idempotency)

**Request:**
```powershell
curl -X POST http://localhost:8080/transfers `
  -H "Content-Type: application/json" `
  -H "Idempotency-Key: 9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d" `
  -d '{"from_account_id": "<SENDER_ID>", "to_account_id": "<RECEIVER_ID>", "amount": 3500, "currency": "USD", "description": "Payment for coffee"}'
```

**Response (`201 Created`):**
```json
{
  "transaction_id": "b2e1d3f4-...",
  "from_account_id": "<SENDER_ID>",
  "to_account_id": "<RECEIVER_ID>",
  "amount": 3500,
  "currency": "USD",
  "description": "Payment for coffee",
  "status": "COMPLETED",
  "created_at": "2026-09-15T01:39:46Z"
}
```

> Note: `amount` is in minor units (`3500` = $35.00). Both accounts must share the same currency.

#### Idempotency Behavior:
1. **First Request:** The transfer executes within an atomic transaction, stores the `idempotency_key` and body SHA-256 fingerprint, and returns `201 Created`.
2. **Safe Replay (Same Key, Same Body):** Returns the original `201 Created` transaction result immediately without double-debiting.
3. **Mismatched Replay (Same Key, Different Body):** Returns `409 Conflict` (`"error": "transfer with this idempotency key already exists"`).
4. **Concurrent Requests:** Handled by PostgreSQL's unique constraint on `idempotency_key` and driver collision recovery (`23505`), preventing race conditions.

---

### Error Responses

All error responses adhere to a consistent JSON format:

```json
{
  "error": "insufficient funds"
}
```

| HTTP Status | Condition | Example Error Message |
|---|---|---|
| `400 Bad Request` | Malformed JSON, unsupported currency, invalid UUID, zero/negative amount, or self-transfer | `"amount must be greater than zero"` / `"cannot transfer to the same account"` |
| `404 Not Found` | Requested account ID does not exist | `"one or both accounts not found"` / `"account not found"` |
| `409 Conflict` | Duplicate account `(owner_id, currency)`, currency mismatch on transfer, or idempotency key reused with different payload | `"accounts must share the same currency"` / `"transfer with this idempotency key already exists"` |
| `422 Unprocessable Entity` | Sender has insufficient funds | `"insufficient funds"` |
| `500 Internal Server Error` | Database error or unexpected server failure (internal details masked) | `"internal server error"` / `"transfer failed"` |