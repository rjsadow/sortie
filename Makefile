.PHONY: all build clean dev dev-backend dev-frontend frontend backend deps kind kind-windows kind-teardown migrate-up migrate-down migrate-status test test-integration test-e2e test-all

all: build

# Install dependencies
deps:
	cd web && npm install

# Build the frontend
frontend: deps
	cd web && npm run build

# Build the Go binary (requires frontend to be built first)
backend: frontend
	go build -o sortie .

# Build everything
build: backend

# Create minimal dist placeholder for Go compilation during dev
web/dist/.placeholder:
	@mkdir -p web/dist
	@echo '<!DOCTYPE html><html><body>Dev placeholder</body></html>' > web/dist/index.html
	@touch web/dist/.placeholder

# Run Go backend in development mode
dev-backend: web/dist/.placeholder
	@echo "Starting backend on http://localhost:8080"
	go run .

# Run frontend dev server with HMR
dev-frontend: deps
	@echo "Starting frontend on http://localhost:5173"
	cd web && npm run dev

# Run full development environment (backend + frontend)
# Use: make dev
# This runs both servers - frontend proxies API calls to backend
dev:
	@echo "=============================================="
	@echo "  Sortie Development Environment"
	@echo "=============================================="
	@echo ""
	@echo "  Frontend: http://localhost:5173  (use this)"
	@echo "  Backend:  http://localhost:8080  (API only)"
	@echo "  Database: SQLite (sortie.db)"
	@echo ""
	@echo "  Press Ctrl+C to stop all servers"
	@echo "=============================================="
	@echo ""
	@mkdir -p web/dist
	@echo '<!DOCTYPE html><html><body>Dev placeholder</body></html>' > web/dist/index.html
	@trap 'kill 0' EXIT; \
		(cd web && npm install --silent && npm run dev) & \
		(sleep 2 && go run .) & \
		wait

# Clean build artifacts
clean:
	rm -rf web/dist web/node_modules sortie sortie.db

# Run the production server
run: build
	./sortie

# Lint frontend code
lint:
	cd web && npm run lint

# Run Go unit tests
test:
	go test -v -race $$(go list ./... | grep -v /tests/)

# Run API integration tests (full HTTP stack with mock runner)
test-integration: frontend
	go test -v -race -timeout 5m ./tests/integration/...

# Run E2E tests against a live Kind cluster
test-e2e:
	go test -v -timeout 10m -count=1 ./tests/e2e/...

# Run unit + integration tests
test-all: test test-integration

# Setup Kind cluster and deploy with Helm
kind:
	@./scripts/kind-setup.sh

# Setup Kind cluster with Windows RDP support for testing
kind-windows:
	@./scripts/kind-setup.sh windows

# Teardown Kind cluster
kind-teardown:
	@./scripts/kind-setup.sh teardown

# Database migrations
migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status
