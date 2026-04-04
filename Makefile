.PHONY: build run test lint vet clean dev db-up db-down migrate-up migrate-down

# --- Variables ---
APP_NAME=mindsync-server
BUILD_DIR=./bin
MAIN_PATH=./cmd/server
GO=go

# --- Build ---
build:
	$(GO) build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)

# --- Run ---
run: build
	$(BUILD_DIR)/$(APP_NAME)

dev:
	$(GO) run $(MAIN_PATH)/main.go

# --- Test ---
test:
	$(GO) test -v -race -count=1 ./...

test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# --- Lint & Vet ---
lint:
	golangci-lint run ./...

vet:
	$(GO) vet ./...

# --- Database ---
db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-logs:
	docker compose logs -f postgres

# --- Migrations ---
migrate-up:
	migrate -path internal/repository/migrations -database "postgres://$${DB_USER}:$${DB_PASSWORD}@$${DB_HOST}:$${DB_PORT}/$${DB_NAME}?sslmode=$${DB_SSLMODE}" up

migrate-down:
	migrate -path internal/repository/migrations -database "postgres://$${DB_USER}:$${DB_PASSWORD}@$${DB_HOST}:$${DB_PORT}/$${DB_NAME}?sslmode=$${DB_SSLMODE}" down 1

# --- Clean ---
clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html

# --- Help ---
help:
	@echo "MindSync AI Server"
	@echo ""
	@echo "Usage:"
	@echo "  make build        Build the server binary"
	@echo "  make run          Build and run the server"
	@echo "  make dev          Run with go run (development)"
	@echo "  make test         Run all tests with race detection"
	@echo "  make test-cover   Run tests with coverage report"
	@echo "  make lint         Run golangci-lint"
	@echo "  make vet          Run go vet"
	@echo "  make db-up        Start PostgreSQL via Docker Compose"
	@echo "  make db-down      Stop PostgreSQL"
	@echo "  make db-logs      Tail PostgreSQL logs"
	@echo "  make migrate-up   Run all pending migrations"
	@echo "  make migrate-down Rollback last migration"
	@echo "  make clean        Remove build artifacts"
