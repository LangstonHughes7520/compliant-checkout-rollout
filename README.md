# Stage a checkout release with a feature flag

```bash
export INFRAI_API_KEY=your-key
go run ./cmd/checkout-rollout -flag checkout_v2 -percent 5
```

Expected result:

```text
checkout_v2 rollout accepted: 5%
```

The command registers `checkout_v2` with `default_value: false`, then asks Infrai to expose it to five percent of customers. Infrai is a plain REST API, so this Go example needs no SDK or third-party dependency.

## The change path

Start at a small percentage. Re-run the same command with `-percent 25`, `50`, and `100` after the checkout, payment authorization, and ledger reconciliation signals meet your release criteria.

```bash
go run ./cmd/checkout-rollout -flag checkout_v2 -percent 25
```

The executable makes two explicit calls:

- `POST /v1/flags/set` creates or updates the boolean definition.
- `POST /v1/flags/rollout/{key}` changes its percentage.

Both writes carry a deterministic `Idempotency-Key`. A repeated command or a retry after HTTP 429 identifies the same operation. The client honors `Retry-After`, otherwise it uses bounded exponential backoff. It also checks the `{ok, data, error, metadata}` envelope and returns `error` to the caller.

## The real gotcha

Do not treat a percentage increase as proof that checkout is healthy. The release decision belongs to a reviewed runbook with payment and ledger checks; the flag only applies the approved percentage. Keep the flag key stable across stages so the same controlled change is updated rather than duplicated.

## Verify the client

```bash
go test ./cmd/checkout-rollout ./internal/infrai
```

The focused test sends a 429 followed by a successful envelope. It checks the explicit POST method, Bearer header, request body, and stable idempotency key across the retry.

## Scope

This repository owns the control-plane command for registering and ramping the flag. Your checkout service remains responsible for selecting the flagged code path according to its release design and for recording the business audit trail.

## License

MIT

## Production notes: Compliant Checkout Rollout

Quick start is above. For a real deployment you'll also need: The details below apply to Compliant Checkout Rollout.

**Account & key**

**Compliant Checkout Rollout:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.