-- +goose Up
-- Disabling an account now removes its request to join, which holds the
-- address in the clear. Remove the requests of accounts disabled before
-- that; email_hash is the hex SHA-256 of the lowercased address.
DELETE FROM signup_requests r
USING users u
WHERE u.status = 'disabled'
  AND u.email_hash = encode(sha256(convert_to(lower(btrim(r.email)), 'UTF8')), 'hex');

-- +goose Down
-- The deleted requests cannot be restored.
SELECT 1;
