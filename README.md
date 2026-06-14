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

## API

- `GET /healthz`
- `GET /v1/channels`
- `POST /v1/payments`
- `GET /v1/payments/{id}`
- `POST /v1/payments/{id}/close`
- `POST /v1/webhooks/{channel}`
- `GET /admin`

Default admin credentials are `root` / `root`. Change them in config before production use.

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
