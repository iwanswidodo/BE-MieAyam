# BE-MieAyam

Backend service for POS Mie Ayam Pak De Surya.

## Stack
- Go HTTP API
- PostgreSQL (integration and migrations are the next foundation step)
- Shared REST API for the Vite web client and React Native Android client

## Run

```bash
go run ./cmd/api
```

Health check: `GET http://localhost:8080/api/health`

## Architecture rules
- Business rules and validation belong in this service.
- Web and Android clients share this API contract.
- Never commit secrets; use environment variables.
- Offline transaction queue belongs to Android; the server is the source of truth after sync.

## Current status
Initial Go service scaffold. PostgreSQL integration, authentication, migrations, and business endpoints are not implemented yet.
