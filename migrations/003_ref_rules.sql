CREATE SCHEMA IF NOT EXISTS ref;

CREATE TABLE IF NOT EXISTS ref.merchant (
  merchant_code TEXT PRIMARY KEY,
  merchant_name TEXT NOT NULL,
  rule_profile JSONB NOT NULL DEFAULT '{}'::jsonb
);

INSERT INTO ref.merchant (merchant_code, merchant_name, rule_profile)
VALUES ('ECI','Electronic City', '{"required_docs":["ktp"],"ktp_regex":"^\\d{16}$"}')
ON CONFLICT (merchant_code) DO NOTHING;
