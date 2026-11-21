# ar-api — the Apply Register back end (stand-in)

A stand-alone Go service with handlers only, for building and testing the Apply Register app end to end. Every
answer comes from memory (seeded at start-up), nothing outlives a restart, and there are no tests, no database and
no cache server. Standard library only.

## Run

```bash
go run .                       # listens on :8090
go run . -addr 127.0.0.1:9000  # or AR_ADDR=127.0.0.1:9000 go run .
```

Go 1.23 or later.

## Sign in (minimal JWT flow)

1. `POST /auth/otp/start` `{"phone": "+15555550100"}` (or `{"email": "…"}`) → `{challenge}`
2. `POST /auth/otp/verify` `{"challenge": "…", "code": "000000"}` → `{accessToken, refreshToken, expiresIn}`

The code is always `000000`. Access tokens are RS256 JWTs (15 minutes) with `sub`, `role`, `org`, `region`,
`segment`; the public key is at `GET /auth/.well-known/jwks.json` (issuer `https://api.apply-register.example/auth`,
audience `apply-register`). `POST /auth/refresh` rotates the refresh token (each works once); `POST /auth/logout`,
`DELETE /auth/account`.

| Phone | Who | Role |
|---|---|---|
| `+15555550100` | Selam Bekele | `member` (has registrations) |
| `+15555550101` | Jonas Lind | `staff` of `bank` |
| `+15555550102` | Ama Owusu | `maintainer` |

Any other phone or e-mail signs up a new member.

## Endpoints

REST under `/v1` (JSON; times ISO 8601; money `{"amount": "12.50", "currency": "EUR"}`):

| Method and path | Notes |
|---|---|
| `GET /categories` | 8 categories |
| `GET /orgs?q&category&near=lat,lng&radiusKm&sort=relevance\|closing\|distance\|name&cursor&limit` | `{items, next, more, total}`; also `lat`, `lng` for the nearby source; 8 organisations and 30 "coming soon" for paging |
| `GET /orgs/{slug}` · `GET /orgs/{slug}/status` | `{open, closesAt, seatsLeft}` |
| `GET /orgs/devconf/products` · `GET /orgs/homes/listings` · `GET /orgs/relief/campaign` | |
| `GET /me` · `GET /me/donations` | auth |
| `GET /me/registrations?status&cursor&limit` or `?offset&pageSize` | auth |
| `GET /me/registrations/{id}` · `DELETE /me/registrations/{id}` (withdraw) | auth |
| `POST /registrations` `{orgSlug, formKey?, values?}` | auth, honours `Idempotency-Key`; `409` for the same form twice |
| `PATCH /registrations/{id}` `{status}` | staff; adds an inbox notice |
| `POST /uploads` (multipart field `file`) | auth, ≤ 10 MiB, PDF or image → `{fileId}` |
| `POST /transfers` `{amount, currency, to: {iban}, reference}` | auth; above 10,000 is refused (to show the error path) |
| `POST /accounts` | auth; Nordvia account opening |
| `GET /staff/orgs/{slug}/stats` · `GET /staff/orgs/{slug}/submissions?status&sort&page&pageSize` | staff of that org, or a maintainer |
| `POST /checkins` `{passCode}` | staff → `{ok, name}`; `409` when already checked in |
| `POST /partner-requests` | header `X-AR-Key: dev-partner-key` |

GraphQL `POST /graphql`: `programmes(q, cursor)`, `programme(id)`, `agenda(day)`, `speakers`, `workshops` (returns
whole objects; the client reads the fields it selected).

Live:

- WebSocket `/ws/races/{raceId}/runners/{bib}` — `{lat, lng, km, etaMinutes, status}` every 3 s.
- SSE `/sse/inbox` (auth) — a `Notice` every 20 s · `/sse/clinics/{id}/queue?ticket` — `{position, waitMinutes,
  status}` every 5 s · `/sse/registrations/{id}` — `submitted → reviewing → approved`.

## Error switches

Send `X-AR-Fail: 500 | 401 | 409 | timeout | slow` on any request to get that failure (`slow` waits 3 s, `timeout`
60 s). CORS is open, for previews.
