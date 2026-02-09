# Build stage: Frontend
FROM node:22-alpine@sha256:48f53c3f0105ccddcc5e4f520347398dfc0ba9b3008fbfd98a2add27e5797957 AS frontend

WORKDIR /app/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# Build stage: Documentation
FROM node:22-alpine@sha256:48f53c3f0105ccddcc5e4f520347398dfc0ba9b3008fbfd98a2add27e5797957 AS docs

WORKDIR /app/docs-site

COPY docs-site/package*.json ./
RUN npm ci

COPY docs-site/ ./
RUN npm run build

# Build stage: Go binary
FROM golang:1.24-alpine@sha256:757779acac4af1b349a20f357c7296097b4a0b89da4ad0e370b339060077282a AS backend

WORKDIR /app

# Copy go module files first for caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code and built frontend + docs
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
COPY --from=docs /app/docs-site/dist ./docs-site/dist

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o sortie .

# Runtime stage: Minimal image
FROM gcr.io/distroless/static-debian12:nonroot@sha256:5d09f5106208a46853a7bebc12c4ce0a2da33f863c45716be11bb4a5b2760e41

COPY --from=backend /app/sortie /sortie

EXPOSE 8080

ENTRYPOINT ["/sortie"]
