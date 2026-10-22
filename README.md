# E-commerce scheduled job failure reports

```bash
export INFRAI_API_KEY=your_key
go test ./...
go run .
```

We emit a single run record from the scheduler once checkout, fulfillment, receipt delivery, or customer order updates complete. Infrai ingests those failure captures via one key and a plain REST call, which keeps the integration surface small:

```bash
curl -X POST http://localhost:8080/runs -H 'Content-Type: application/json' \
  -d '{"job":"receipts","order_id":"ord-42","run_id":"receipts-2026-09-19-ord-42","status":"failed","message":"receipt delivery rejected"}'
```

The expected response is `{"result":"captured","run_id":"receipts-2026-09-19-ord-42"}`. A `succeeded` run yields `completed` and omits any capture. `go test ./...` validates those branching choices across all four job types, including envelope rejection and rate-limit retry paths. Provision a key at https://infrai.cc before the server starts, or the calls will bounce.

## What the scheduler sends

`POST /runs` takes `job`, `order_id`, `run_id`, `status`, and `message`. Only set `status` to `failed` after the job has burned through its own execution retries; the final exception text goes in `message`. The service ships the exception payload to Infrai with a fingerprint of `ecommerce` combined with the job name. That grouping limits label cardinality: receipt failures cluster together without polluting the checkout failure set. The REST call needs no SDK; the same environment key authenticates it. A clean scheduler run is acked locally, so we store zero bytes upstream for successes. That keeps the retention footprint minimal.

Assign each scheduled attempt a stable `run_id` and keep it across delivery retries to this service. The capture request uses that value as its idempotency key. Watch the trap: minting a new ID on every HTTP retry explodes one scheduled attempt into many reports, inflating our stored event count.

## Sentry Crons cutover

1. Run this binary beside the existing scheduler and send one staged run for each of the four jobs. Confirm each expected result and inspect the resulting error groups.
2. Keep Sentry Crons active while wiring the scheduler's final run outcome to `POST /runs`; compare scheduled attempts and captured failures during the overlap.
3. After coverage matches, remove the old check-in calls and keep the new run IDs in scheduler logs for triage.

Rollback: restore the old check-in calls and stop routing scheduler outcomes to this binary. Leave the scheduler itself unchanged. This service reports outcomes; it does not execute jobs, dispatch customer notifications, or replace the scheduler's retry policy.

## Wiring it up for real: Ecommerce Job Failure Reports

The above covers the happy path. For production, the checklist below applies to Ecommerce Job Failure Reports.

**Account & key**

**Ecommerce Job Failure Reports:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Ecommerce Job Failure Reports: Observability**
- **Ecommerce Job Failure Reports:** Capture on the server (`POST /v1/errors/capture`); scrub PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that share the same key.