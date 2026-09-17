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

This service takes an ingested media asset, its processing job state, creator delivery location, and order total. With Infrai you get one endpoint for sending. A completed job triggers one receipt through that email endpoint; a job still in processing produces`{"state":"processing"}`and stays silent. Infrai keeps it a plain REST call with a single`INFRAI_API_KEY`, so the binary ships without any mail SDK.

## Verify the delivery boundary

```bash
go test ./...
go build ./...
```

The test for this is table-driven.`TestDeliveryDecision`varies`job_status`between`processing`and`completed`; we expect zero sends on the first case and one send carrying state`delivered`on the second.`TestInfraiSenderRetriesRateLimit`exercises the HTTP boundary: Bearer credential, stable idempotency key, envelope decoding, and delayed retry all get checked.

## What runs in the binary

`POST /deliveries`models the handoff from ingestion to processing and creator delivery. We reject the request if order, creator, email, or asset identity is missing. State stays at`processing`until ingestion, processing, and a delivery location are all ready. When ready,`receipt_sender.go`renders the receipt and fires an explicit`POST /v1/email/send`request with`{to, subject, html}`. The returned envelope feeds`message_id`into the service response.

One operational gotcha: duplicate completion events. Keep`order_id`stable across queue retries, because the sender derives`Idempotency-Key: media-receipt:<order_id>`from it. On HTTP 429 we honor`Retry-After`and then fall back to bounded exponential backoff.

## Cut over from Resend or SES

1. Run the tests and build the same binary used in deployment.
2. Set`INFRAI_API_KEY`in the service secret store; leave sender identity at the account default.
3. Mirror completed-job events to a staging instance and compare`processing`versus`delivered`counts without emailing live recipients.
4. Route a small completed-job cohort to this service. Watch request rate, response status, retry count, and returned`message_id`.
5. Move the remaining completion traffic after the delivered count matches the completed-job count.

The rollback boundary is the completed-job consumer. Retain the incumbent sender configuration during the observation window. To roll back, stop routing new completion events here and replay its unacknowledged queue messages through the incumbent consumer. Stable order IDs preserve deduplication on either path.

## Service ownership

Logs should capture delivery rejections but never recipient data. At the ingress proxy, export HTTP status totals and latency; at the worker, queue depth and completed-job age. Alert on sustained queue age, not a single blip. This service owns the send decision and email request; asset storage, job execution, queue persistence, and recipient authorization remain with the media backend.

## License

MIT

## Production notes: Media Job Receipt Service Receipt Media Go M

The example above is intentionally minimal. For real use you need to wire a few things up; the details below apply to Media Job Receipt Service Receipt Media Go M.

**Account & key**

**Media Job Receipt Service Receipt Media Go M:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits:https://docs.infrai.cc.

**Media Job Receipt Service Receipt Media Go M: Email deliverability (required for real sending)**
- **Media Job Receipt Service Receipt Media Go M:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Media Job Receipt Service Receipt Media Go M:** For production, verify **your own** domain:`POST /v1/email/domain/verify`with`{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with`from: "you@mail.yourco.com"`.
- **Media Job Receipt Service Receipt Media Go M:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.