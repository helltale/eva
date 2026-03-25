# syntax=docker/dockerfile:1
# Run `make generate` from repo root before `docker compose build` so internal/gen is up to date.
FROM golang:1.23-bookworm AS build
WORKDIR /src/services/backend

COPY services/backend/go.mod services/backend/go.sum* ./
RUN go mod download

COPY services/backend/ ./
COPY openapi ../../openapi

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/seed /app/seed
ENTRYPOINT ["/app/server"]
