# Build stage: Frontend
FROM node:22-alpine@sha256:e4bf2a82ad0a4037d28035ae71529873c069b13eb0455466ae0bc13363826e34 AS frontend

WORKDIR /app/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# Build stage: Documentation
FROM node:22-alpine@sha256:e4bf2a82ad0a4037d28035ae71529873c069b13eb0455466ae0bc13363826e34 AS docs

WORKDIR /app/docs-site

COPY docs-site/package*.json ./
RUN npm ci

COPY docs-site/ ./
RUN npm run build

# Build stage: Go binary
FROM golang:1.24-alpine@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 AS backend

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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:a9329520abc449e3b14d5bc3a6ffae065bdde0f02667fa10880c49b35c109fd1

COPY --from=backend /app/sortie /sortie

EXPOSE 8080

ENTRYPOINT ["/sortie"]
