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
FROM golang:1.25-alpine@sha256:f6751d823c26342f9506c03797d2527668d095b0a15f1862cddb4d927a7a4ced AS backend

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
