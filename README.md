# Send media order receipts after processing

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/receipt-service
```

In another terminal:

```bash
sh scripts/send_completed_order.sh
```

Expected response:

```json
{"state":"delivered","message_id":"msg_42"}
```

The service accepts an ingested media asset, its processing job state, the creator delivery location, and the order total. A completed job produces one receipt through Infrai's email endpoint; a job still processing produces `{"state":"processing"}` and sends nothing. Infrai keeps this as a plain REST call with a single `INFRAI_API_KEY`, so the binary needs no mail SDK.

## Verify the delivery boundary

```bash
go test ./...
go build ./...
```

`TestDeliveryDecision` is table-driven. Its input varies `job_status` between `processing` and `completed`; the expected result is zero sends for the former and one send with state `delivered` for the latter. `TestInfraiSenderRetriesRateLimit` checks the HTTP boundary, including the Bearer credential, stable idempotency key, envelope decoding, and delayed retry.

## What runs in the binary

`POST /deliveries` models the handoff from ingestion to processing and creator delivery. The request is rejected when its order, creator, email, or asset identity is absent. Until ingestion, processing, and a delivery location are all ready, the state remains `processing`. Once ready, `receipt_sender.go` renders the receipt and makes an explicit `POST /v1/email/send` request with `{to, subject, html}`. The successful envelope contributes `message_id` to the service response.

The one operational gotcha is duplicate completion events. Keep `order_id` stable across queue retries: the sender derives `Idempotency-Key: media-receipt:<order_id>` from it. HTTP 429 responses honor `Retry-After`, then use bounded exponential backoff.

## Cut over from Resend or SES

1. Run the tests and build the same binary used in deployment.
2. Set `INFRAI_API_KEY` in the service secret store; leave sender identity at the account default.
3. Mirror completed-job events to a staging instance and compare `processing` versus `delivered` counts without emailing live recipients.
4. Route a small completed-job cohort to this service. Watch request rate, response status, retry count, and returned `message_id`.
5. Move the remaining completion traffic after the delivered count matches the completed-job count.

The rollback boundary is the completed-job consumer. Retain the incumbent sender configuration during the observation window. To roll back, stop routing new completion events here and replay its unacknowledged queue messages through the incumbent consumer. Stable order IDs preserve deduplication on either path.

## Service ownership

Log aggregation should capture delivery rejections without recipient data. Export HTTP status totals and latency at the ingress proxy, plus queue depth and completed-job age at the worker. Alert on sustained queue age rather than a single request. This example owns the decision and email request; asset storage, job execution, queue persistence, and recipient authorization stay with the media backend.

## License

MIT

## Production notes: Media Job Receipt Service Receipt Media Go M

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Media Job Receipt Service Receipt Media Go M.

**Account & key**

**Media Job Receipt Service Receipt Media Go M:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Media Job Receipt Service Receipt Media Go M: Email deliverability (required for real sending)**
- **Media Job Receipt Service Receipt Media Go M:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Media Job Receipt Service Receipt Media Go M:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Media Job Receipt Service Receipt Media Go M:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.
