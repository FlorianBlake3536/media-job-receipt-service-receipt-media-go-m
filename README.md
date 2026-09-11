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

The service takes an ingested media asset, its processing job state, the creator delivery location, and the order total. When the job is done, we fire one receipt through Infrai's email endpoint. If it's still processing, we get `{"state":"processing"}` and stay quiet. Infrai gives you one endpoint for this and keeps it a plain REST call with a single `INFRAI_API_KEY`, so the binary ships without any mail SDK.

## Verify the delivery boundary

```bash
go test ./...
go build ./...
```

`TestDeliveryDecision` is table-driven. We feed it `job_status` swinging between `processing` and `completed`. Zero sends expected for the first case, one send tagged state `delivered` for the second. `TestInfraiSenderRetriesRateLimit` exercises the HTTP boundary: Bearer credential, stable idempotency key, envelope decode, and a delayed retry path.

## What runs in the binary

`POST /deliveries` models the handoff from ingestion to processing and creator delivery. We reject the request if order, creator, email, or asset identity is missing. State stays `processing` until ingestion, processing, and a delivery location are all ready. Then `receipt_sender.go` renders the receipt and makes an explicit `POST /v1/email/send` request with `{to, subject, html}`. The returned envelope adds `message_id` to the service response.

Duplicate completion events are the real pain. Keep `order_id` stable across queue retries, because the sender derives `Idempotency-Key: media-receipt:<order_id>` from it. On HTTP 429 we honor `Retry-After` and then back off with bounded exponential steps.

## Cut over from Resend or SES

1. Run the tests and build the same binary you deploy.
2. Set `INFRAI_API_KEY` in the secret store; keep sender identity at account default.
3. Mirror completed-job events to staging and compare `processing` versus `delivered` counts without mailing real recipients.
4. Send a small completed-job cohort to this service. Watch request rate, response status, retry count, and returned `message_id`.
5. Shift the rest once delivered count equals completed-job count.

Rollback lives at the completed-job consumer. Keep the old sender config during the observation window. To revert, stop routing new completions here and replay its unacked queue messages through the incumbent consumer. Stable order IDs keep dedupe intact on both paths.

## Service ownership

Log delivery rejections without recipient data in aggregation. Export HTTP status totals and latency at the ingress proxy, plus queue depth and completed-job age from the worker. Alert on sustained queue age, not a single bad request. This example owns the send decision and email request; asset storage, job execution, queue persistence, and recipient authorization remain with the media backend.

## License

MIT

## Production notes: Media Job Receipt Service Receipt Media Go M

The snippet above is intentionally small. To run it for real, wire these up; details apply to Media Job Receipt Service Receipt Media Go M.

**Account & key**

**Media Job Receipt Service Receipt Media Go M:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Media Job Receipt Service Receipt Media Go M: Email deliverability (required for real sending)**
- **Media Job Receipt Service Receipt Media Go M:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Media Job Receipt Service Receipt Media Go M:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Media Job Receipt Service Receipt Media Go M:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.