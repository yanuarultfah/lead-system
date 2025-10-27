# Leads System – Full Bundle

Fitur:
- Gin API + SOLID-ish layers
- QDE → FDE → Scoring Callback → Approval → Order/Disbursement
- Idempotency header + table
- Outbox pattern + Worker
- Kafka (Confluent) + Schema Registry (JSON/Avro)
- Stricter validations: Email, Phone (ID), NPWP; FDE rules per merchant
- Postgres RLS + PII masked view
- Gateway rate-limiter examples (NGINX, Kong, Istio)

## Migrasi
psql "$PG_URI" -f migrations/001_init.sql
psql "$PG_URI" -f migrations/002_extend.sql
psql "$PG_URI" -f migrations/003_ref_rules.sql
psql "$PG_URI" -f migrations/004_pii_rls.sql

## Jalankan
make run          # API
make worker       # Outbox Kafka publisher (CGO & librdkafka required)
