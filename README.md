# Package Tracking – Backend (Go + MySQL, modular monolith)

One API server (port 8080) made of three independent modules.

```
cmd/server/main.go        wires the modules together
internal/common/          config, DB, JWT auth, JSON helpers, shared order queries
internal/user/            #1 Register, #2 Login, courier management (admin)
internal/order/           #4 Create, #5 My orders, #6 Details + cancel, #9 Admin manage
internal/courier/         #7 Assigned + accept/decline, #8 Update status, #10 Assign/reassign
db/init.sql               MySQL schema
```
Each module exposes `Register(mux, db)` and owns its handlers and routes, so it can be split
into a separate service later.

## Run
```bash
docker compose up --build
```
Seeded accounts: `admin@example.com / Admin@123`, `courier@example.com / Courier@123`.

Without Docker: start MySQL, run `db/init.sql`, `go mod tidy`, then
`go run ./cmd/server` with env `DB_DSN`, `JWT_SECRET`, `PORT` (default 8080).

Database created before the customer columns existed? Run the migration once:
`docker exec -i tracker-mysql mysql -uroot -proot < db/migrations/001_customer_and_collect.sql`
(or `docker compose down -v` to start the compose database from scratch).

## Auth
`POST /api/auth/login` returns `{token, user}`. Send `Authorization: Bearer <token>` afterwards.
Roles: `user`, `courier`, `admin`.

## Order status flow
`pending → assigned → accepted → picked_up → in_transit → delivered`
Customer can cancel only while `pending`; courier decline returns the order to `pending`;
admin assign/reassign sets `assigned`.

## Endpoints (all on http://localhost:8080)
| Method | Path | Role |
|---|---|---|
| POST | /api/auth/register | public |
| POST | /api/auth/login | public |
| GET | /api/users/me | any |
| GET/POST | /api/users/couriers | admin |
| POST | /api/orders | user |
| GET | /api/orders/my | user |
| GET | /api/orders/{id} | owner / assigned courier / admin |
| PATCH | /api/orders/{id}/cancel | user |
| GET | /api/admin/orders | admin |
| PUT/DELETE | /api/admin/orders/{id} | admin |
| GET | /api/couriers/{id}/orders | courier (self) / admin |
| POST | /api/courier/orders/{id}/accept | courier |
| POST | /api/courier/orders/{id}/decline | courier |
| PATCH | /api/courier/orders/{id}/status `{"status":"picked_up"}` | courier |
| GET | /api/admin/assignments | admin |
| PUT | /api/admin/orders/{id}/assign `{"courier_id":2}` | admin |

Create-order body (all required except `collect_amount`, which defaults to 0):
```json
{
  "pickup_location": "12 Abbas El Akkad St, Nasr City, Cairo",
  "dropoff_location": "5 Mourad St, Dokki, Giza",
  "customer_name": "Mariam Adel",
  "customer_phone": "01112345678",
  "package_details": "2 cotton shirts (1.5 kg)",
  "delivery_time": "2026-11-12T12:00",
  "delivery_fee": 60,
  "collect_amount": 1250
}
```
`customer_name` 2-100 characters, `customer_phone` 8-15 digits (optional leading +), money not negative.
Order responses also carry `seller: {name, phone}` so the courier can reach the seller for pickup.

Frontend (React): set `VITE_API_URL=http://localhost:8080/api` in the frontend's `.env.local`.
Postman: import `postman/PackageTracking.postman_collection.json` (login requests save tokens).
