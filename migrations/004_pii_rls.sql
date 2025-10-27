CREATE SCHEMA IF NOT EXISTS util;

CREATE OR REPLACE FUNCTION util.mask_email(v TEXT) RETURNS TEXT LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE WHEN v IS NULL THEN NULL
              WHEN position('@' in v) = 0 THEN '***@***'
              ELSE substr(v,1,1) || '***' || substr(v, position('@' in v)) END;
$$;

CREATE OR REPLACE FUNCTION util.mask_phone(v TEXT) RETURNS TEXT LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE WHEN v IS NULL THEN NULL ELSE regexp_replace(v, '^(\+?62|0)8[0-9]{4})([0-9]+)$', '\1****') END;
$$;

DO $$ BEGIN
  ALTER TABLE party.customer ADD COLUMN IF NOT EXISTS email_norm TEXT;
  ALTER TABLE party.customer ADD COLUMN IF NOT EXISTS mobile_norm TEXT;
  ALTER TABLE party.customer ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
  ALTER TABLE party.customer ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
EXCEPTION WHEN duplicate_column THEN NULL; END $$;

UPDATE party.customer SET
  email_norm = lower(email),
  mobile_norm = regexp_replace(coalesce(mobile_no,''), '\\D', '', 'g');

CREATE INDEX IF NOT EXISTS idx_customer_email_norm ON party.customer(email_norm);
CREATE INDEX IF NOT EXISTS idx_customer_mobile_norm ON party.customer(mobile_norm);

CREATE OR REPLACE VIEW party.v_customer_masked WITH (security_barrier=true) AS
SELECT
  customer_id, full_name,
  util.mask_phone(mobile_no) AS mobile_no,
  util.mask_email(email) AS email,
  created_at, updated_at
FROM party.customer;

DO $$ BEGIN
  CREATE ROLE app_writer NOINHERIT;
  CREATE ROLE app_reader NOINHERIT;
  CREATE ROLE analyst_readonly NOINHERIT;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

ALTER TABLE party.customer ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS customer_rw ON party.customer;
CREATE POLICY customer_rw ON party.customer FOR ALL TO app_writer USING (true) WITH CHECK (true);

DROP POLICY IF EXISTS customer_r ON party.customer;
CREATE POLICY customer_r ON party.customer FOR SELECT TO app_reader USING (true);

REVOKE ALL ON party.customer FROM analyst_readonly;
GRANT SELECT ON party.v_customer_masked TO analyst_readonly;
REVOKE SELECT ON party.customer FROM PUBLIC;
