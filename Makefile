.PHONY: all build clean dev dev-backend dev-frontend dev-docs frontend backend deps docs-deps docs kind kind-windows kind-teardown migrate-up migrate-down migrate-status test test-integration test-e2e test-all test-postgres test-integration-postgres playwright-install test-playwright test-playwright-ui test-playwright-report test-helm

all: build

# Install dependencies
deps:
	cd web && npm install

# Install docs dependencies
docs-deps:
	cd docs-site && npm install

# Build the frontend
frontend: deps
	cd web && npm run build

# Build the docs site
docs: docs-deps
	cd docs-site && npm run build

# Build the Go binary (requires frontend and docs to be built first)
backend: frontend docs
	go build -o sortie .

# Build everything
build: backend

# Create minimal dist placeholder for Go compilation during dev
web/dist/.placeholder:
	@mkdir -p web/dist
	@echo '<!DOCTYPE html><html><body>Dev placeholder</body></html>' > web/dist/index.html
	@touch web/dist/.placeholder

# Create minimal docs-site dist placeholder for Go compilation during dev
docs-site/dist/.placeholder:
	@mkdir -p docs-site/dist
	@touch docs-site/dist/.placeholder

# Run Go backend in development mode
dev-backend: web/dist/.placeholder docs-site/dist/.placeholder
	@echo "Starting backend on http://localhost:8080"
	go run .

# Run frontend dev server with HMR
dev-frontend: deps
	@echo "Starting frontend on http://localhost:5173"
	cd web && npm run dev

# Run docs dev server
dev-docs: docs-deps
	@echo "Starting docs on http://localhost:5174"
	cd docs-site && npm run dev

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
	@mkdir -p docs-site/dist
	@touch docs-site/dist/.placeholder
	@trap 'kill 0' EXIT; \
		(cd web && npm install --silent && npm run dev) & \
		(sleep 2 && go run .) & \
		wait

# Clean build artifacts
clean:
	rm -rf web/dist web/node_modules docs-site/dist docs-site/node_modules docs-site/.vitepress/cache sortie sortie.db

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

# Run Go unit tests against Postgres (requires SORTIE_TEST_POSTGRES_DSN)
test-postgres:
	SORTIE_TEST_DB_TYPE=postgres go test -v -race -p 1 -count=1 $$(go list ./... | grep -v /tests/)

# Run API integration tests against Postgres (requires SORTIE_TEST_POSTGRES_DSN)
test-integration-postgres: frontend
	SORTIE_TEST_DB_TYPE=postgres go test -v -race -p 1 -timeout 5m -count=1 ./tests/integration/...

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

# Playwright E2E tests
playwright-install:
	web/node_modules/.bin/playwright install --with-deps chromium firefox webkit

test-playwright: build
	web/node_modules/.bin/playwright test --config web/playwright.config.ts

test-playwright-ui: build
	web/node_modules/.bin/playwright test --config web/playwright.config.ts --ui

test-playwright-report:
	web/node_modules/.bin/playwright show-report web/playwright-report

# Helm chart unit tests (requires helm-unittest plugin)
test-helm:
	helm unittest charts/sortie
