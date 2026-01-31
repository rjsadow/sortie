# Build stage: Frontend
FROM node:22-alpine AS frontend

WORKDIR /app/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# Build stage: Go binary
FROM golang:1.24-alpine AS backend

WORKDIR /app

# Copy go module files first for caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code and built frontend
COPY . .
COPY --from=frontend /app/web/dist ./web/dist

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o launchpad .

# Runtime stage: Minimal image
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=backend /app/launchpad /launchpad

EXPOSE 8080

ENTRYPOINT ["/launchpad"]
