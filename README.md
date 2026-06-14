# pay233-server

`pay233-server` is the unified payment center for company products. It exposes a single HTTP API and routes payment requests to any channel configured in `config.example.json`.

## Run

```bash
go run ./cmd/pay233-server -config config.example.json
```

Or run it from the umbrella repository:

```bash
docker compose up --build
```

The default listen address is `:5500`.

## Logs

`pay233-server` writes daily rotated JSON logs with 31-day retention by default:

- `logs/app-YYYY-MM-DD.log`: application `slog` output
- `logs/payments/payment-YYYY-MM-DD.log`: payment audit events such as create, close, webhook, and manual lost-order marking

Configure the directory and retention:

```json
{
  "logging": {
    "dir": "logs",
    "retention_days": 31
  }
}
```

## Storage

Payments, admin users, and audit logs are persisted by default:

```json
{
  "storage": {
    "payments_path": "data/payments.jsonl",
    "admin_users_path": "data/admin-users.json",
    "audit_path": "data/audit.jsonl",
    "audit_retention_days": 31
  }
}
```

The server replays payment and audit files on startup, so orders and operation history survive process restarts. Back up these files together with daily payment logs. For larger deployments, keep the store interfaces and replace the file stores with PostgreSQL, MySQL, or another transactional backend.

## Release

Pushing a tag like `v0.1.0` builds GitHub Release assets:

- `pay233-server-linux-amd64`
- `pay233-server-linux-arm64`
- `pay233-server-darwin-amd64`
- `pay233-server-darwin-arm64`
- `pay233-server-windows-amd64.exe`
- `pay233-server-windows-arm64.exe`

## Test

```bash
go test -cover ./...
go vet ./...
```

From the umbrella repository, run the full local verification set:

```bash
make verify
```

## API

- `GET /healthz`
- `GET /v1/channels`
- `POST /v1/payments`
- `GET /v1/payments/{id}`
- `POST /v1/payments/{id}/close`
- `POST /v1/webhooks/{channel}`
- `GET /admin`
- `GET /admin/login.html`
- `GET /admin/dashboard.html`

Default admin credentials are `root` / `root`. Change them in config before production use.

## Admin Roles and Audit Logs

The admin console supports three roles:

- `root`: full access, can create/delete `admin` and `employee` accounts, and can prune expired audit logs.
- `admin`: can operate payments, mark lost orders, and retry merchant callbacks.
- `employee`: read-only access to dashboards, payment lists, and audit logs.

All login, account, payment-operation, callback-retry, and audit-prune actions are written to the append-only audit log. Audit logs are retained for 31 days by default. There is no arbitrary audit deletion API; only `root` can trigger retention pruning for expired entries.

## Channel Health Monitoring

The server checks downstream payment channel health automatically. Default settings:

```json
{
  "monitor": {
    "channel_health_interval_seconds": 60,
    "channel_health_timeout_seconds": 5
  }
}
```

Each enabled channel can expose a health endpoint through `options.health_url`:

```json
{
  "name": "wechat",
  "provider": "wechat_pay",
  "enabled": true,
  "options": {
    "health_url": "https://api.mch.weixin.qq.com/health"
  }
}
```

The dashboard shows each channel's latest health, latency, last check time, and error. `root` and `admin` can trigger an immediate health check from the dashboard; `employee` can only view the result. Automatic checks write audit records when a channel becomes unhealthy or changes status.

## Test and Release Environments

`POST /v1/payments` supports `envType` on every request:

```json
{
  "envType": "test",
  "merchant_id": "merchant_1",
  "out_trade_no": "order_10001",
  "channel": "mock",
  "amount": { "currency": "CNY", "amount": 100 },
  "subject": "Test order"
}
```

Use `test` for sandbox traffic and `release` for formal payment traffic. Missing values default to `test`; `env_type` is accepted as an alias. The order index includes the environment, so the same merchant order number can be tested and then released on the same server without collision.

Webhook payloads also accept `envType` / `env_type`. The admin APIs support `?envType=test`, `?envType=release`, and `?envType=all`; the dashboard UI exposes the same switch.

## Merchant Callbacks

When a payment has `notify_url`, provider webhooks trigger a POST to the merchant callback URL. The body is the latest payment JSON, signed with the same headers used by API requests:

- `X-Pay233-Timestamp`
- `X-Pay233-Signature`

The signature is `hex(hmac_sha256(secret, timestamp + "." + body))`. Non-2xx merchant responses are recorded as callback failures and shown in the admin dashboard. Admins can retry failed callbacks from the abnormal payment table.

## Built-In Channels

The first release includes a unified provider adapter layer for:

- WeChat Pay
- Alipay
- Stripe
- PayPal
- Google Pay
- Apple Pay / Apple iOS in-app purchase
- UnionPay
- Generic third-party providers

Provider implementations currently share the same adapter contract and mockable creation/webhook flow. Real gateway SDK signing, certificate verification, refund capture, and settlement reconciliation can be added provider by provider behind the existing interface.

Signed API requests use:

- `X-Pay233-Timestamp`
- `X-Pay233-Signature`

The signature is `hex(hmac_sha256(secret, timestamp + "." + body))`.

## Add a Channel

Add a channel to the config:

```json
{
  "name": "wechat-prod",
  "provider": "mock",
  "enabled": true
}
```

Then register the provider implementation in `internal/payment/registry.go`. Once a channel is configured and enabled, clients can pass its `name` in `POST /v1/payments`.
