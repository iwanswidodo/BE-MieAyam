# BE-MieAyam

Backend service for POS Mie Ayam Pak De Surya.

## Run

Set `DATABASE_URL` to a PostgreSQL connection string, then run:

```bash
export DATABASE_URL="postgres://user:password@localhost:5432/mieayam?sslmode=disable"
export APP_ADDR=":8080"
export UPLOAD_DIR="uploads"
export CORS_ORIGIN="http://localhost:5173"
go run ./cmd/api
```

The service checks PostgreSQL connectivity and creates the `products` table on startup.

## Product API

- `GET /api/products`: list active products. Add `?all=true` to include inactive products; optional `?category=...`.
- `POST /api/products`: create a product.
- `GET /api/products/{id}`: product detail.
- `PUT /api/products/{id}`: update a product.
- `DELETE /api/products/{id}`: soft-delete/deactivate a product.
- `POST /api/products/{id}/image`: upload JPG/PNG image using multipart field `image`, max 5 MB.
- `GET /uploads/{filename}`: serve uploaded product images.
- `GET /api/health`: application and database health.

Example JSON:

```json
{"name":"Mie Ayam Original","category":"Mie Ayam","variant":"Normal","price":17000,"active":true}
```

Dine-in and take-away share one price. Each sellable variant is a product row. Product images are optional and can be uploaded later. Persist and back up `UPLOAD_DIR`.
