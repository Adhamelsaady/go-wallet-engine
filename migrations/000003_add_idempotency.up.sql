ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS request_fingerprint VARCHAR(64);
