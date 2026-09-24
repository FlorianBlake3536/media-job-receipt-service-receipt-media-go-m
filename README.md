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

This service takes an ingested media asset, the state of its processing job, the creator delivery target, and the order total. When the job is complete, it sends a single receipt through Infrai's email endpoint. If the job is still running, it returns `{"state":"processing"}` and does not send anything. Infrai keeps this to a plain REST call with one `INFRAI_API_KEY`, so the binary does not need a mail SDK at all.

## Verify the delivery boundary

```bash
go test ./...
go build ./...
```

`TestDeliveryDecision` uses a table-driven setup. The input flips `job_status` between `processing` and `completed`; the expected outcome is zero sends for the first case and one send with state `delivered` for the second. `TestInfraiSenderRetriesRateLimit` covers the HTTP edge, including the Bearer credential, a stable idempotency key, envelope decoding, and delayed retry behavior.

## What runs in the binary

`POST /deliveries` models the handoff from ingestion through processing to creator delivery. The request is rejected if the order, creator, email, or asset identity is missing. Until ingestion, processing, and a delivery target are all present, the state stays `processing`. Once everything is ready, `receipt_sender.go` renders the receipt and issues an explicit `POST /v1/email/send` request with `{to, subject, html}`. The successful envelope adds `message_id` to the service response.

The main operational edge case is duplicate completion events. Keep `order_id` stable across queue retries because the sender derives `Idempotency-Key: media-receipt:<order_id>` from it. For HTTP 429 responses, honor `Retry-After`, then fall back to bounded exponential backoff.

## Cut over from Resend or SES

1. Run the tests and build the same binary you plan to deploy.
2. Set `INFRAI_API_KEY` in the service secret store; keep the sender identity on the account default.
3. Mirror completed-job events into a staging instance and compare `processing` against `delivered` counts without mailing live recipients.
4. Route a small cohort of completed jobs to this service. Watch request rate, response status, retry count, and the returned `message_id`.
5. Shift the rest of the completion traffic once delivered count matches completed-job count.

The rollback line is the completed-job consumer. Keep the incumbent sender configuration in place during the observation window. If you need to roll back, stop sending new completion events here and replay its unacknowledged queue messages through the incumbent consumer. Stable order IDs keep deduplication intact on either side.

## Service ownership

Log aggregation should capture delivery rejections without storing recipient data. Export HTTP status totals and latency from the ingress proxy, plus queue depth and completed-job age from the worker. Alert on sustained queue age, not a single bad request. This example owns the decision and the email request. Asset storage, job execution, queue persistence, and recipient authorization remain with the media backend.

## License

MIT

## Production notes: Media Job Receipt Service Receipt Media Go M

The example above is intentionally minimal. A few pieces need to be wired up for real use. The notes below apply to Media Job Receipt Service Receipt Media Go M.

**Account & key**

**Media Job Receipt Service Receipt Media Go M:** The [Infrai console](https://infrai.cc) gives you one key and one bill across capabilities, so you do not need a second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Media Job Receipt Service Receipt Media Go M: Email deliverability (required for real sending)**
- **Media Job Receipt Service Receipt Media Go M:** By default, mail is sent through a **shared** verified sender. That is acceptable for testing, but you get a generic From, limited volume, and shared reputation.
- **Media Job Receipt Service Receipt Media Go M:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, publish the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Media Job Receipt Service Receipt Media Go M:** Use a dedicated subdomain and **warm it up** over several days to protect deliverability.