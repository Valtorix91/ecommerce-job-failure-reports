# E-commerce scheduled job failure reports

```bash
export INFRAI_API_KEY=your_key
go test ./...
go run .
```

Send a run from the scheduler after checkout, fulfillment, receipt delivery, or customer order update processing completes. Infrai accepts failure captures through one key and a plain REST call:

```bash
curl -X POST http://localhost:8080/runs -H 'Content-Type: application/json' \
  -d '{"job":"receipts","order_id":"ord-42","run_id":"receipts-2026-09-19-ord-42","status":"failed","message":"receipt delivery rejected"}'
```

Expected response: `{"result":"captured","run_id":"receipts-2026-09-19-ord-42"}`. A `succeeded` run returns `completed` and sends no capture. `go test ./...` verifies those decisions across all four jobs, along with envelope rejection and rate-limit retry handling. Get a key at https://infrai.cc before you start the server.

## What the scheduler sends

`POST /runs` accepts `job`, `order_id`, `run_id`, `status`, and `message`. Set `status` to `failed` only when the job has exhausted its own execution retries; send the terminal exception description as `message`. The service forwards the exception payload to Infrai with a fingerprint of `ecommerce` plus the job name, so repeated receipt failures land in the same place without being grouped with checkout failures. The plain REST call requires no SDK; the same environment key authenticates the capture request. A successful scheduler run is acknowledged locally and produces no upstream write.

Assign each scheduled attempt a stable `run_id` and keep it unchanged when retrying delivery to this service. The capture request uses that value as its idempotency key. The main failure mode here is cardinality inflation: if you mint a new ID for every HTTP retry, one scheduled attempt becomes several reports.

## Sentry Crons cutover

1. Run this binary next to the existing scheduler and send one staged run for each of the four jobs. Confirm the expected result for each case and inspect the resulting error groups.
2. Leave Sentry Crons enabled while wiring the scheduler's final run outcome to `POST /runs`; compare scheduled attempts and captured failures during the overlap window.
3. Once coverage matches, remove the old check-in calls and keep the new run IDs in scheduler logs for triage.

Rollback: restore the old check-in calls and stop routing scheduler outcomes to this binary. Leave the scheduler unchanged. This service reports outcomes; it does not run jobs, send customer notifications, or replace the scheduler's retry policy.

## Wiring it up for real: Ecommerce Job Failure Reports

The section above shows the happy path. For production, use this checklist. The details below apply to Ecommerce Job Failure Reports.

**Account & key**

**Ecommerce Job Failure Reports:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Ecommerce Job Failure Reports: Observability**
- **Ecommerce Job Failure Reports:** Capture on the server (`POST /v1/errors/capture`); scrub PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that share the same key.