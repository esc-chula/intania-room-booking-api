-- +goose Up
ALTER TABLE users ADD COLUMN password_hash text;
-- Existing normalized-email collisions fail this migration; never merge accounts.
CREATE UNIQUE INDEX users_email_normalized_idx ON users (lower(btrim(email)));

-- +goose Down
DROP INDEX users_email_normalized_idx;
ALTER TABLE users DROP COLUMN password_hash;
