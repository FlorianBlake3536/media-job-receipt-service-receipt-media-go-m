#!/bin/sh
set -eu

curl --fail-with-body --request POST http://localhost:8080/deliveries \
  --header 'Content-Type: application/json' \
  --data '{
    "order_id": "ord-42",
    "creator": "Mina Chen",
    "email": "creator@example.com",
    "asset_id": "asset-9",
    "ingested": true,
    "job_status": "completed",
    "deliverable": "s3://creator-delivery/asset-9/master.mp4",
    "amount_cents": 2400
  }'
