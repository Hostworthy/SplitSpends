-- +goose Up
-- The storage hash of the invite link a login code was requested with, so
-- the link is checked again when the code is verified. Verification applies
-- the sign-up rules to every code, so codes no longer record whether they
-- were sent to someone waiting for approval.
ALTER TABLE login_challenges ADD COLUMN invite_hash text NOT NULL DEFAULT '';
ALTER TABLE login_challenges DROP COLUMN pending;

-- +goose Down
ALTER TABLE login_challenges ADD COLUMN pending boolean NOT NULL DEFAULT false;
ALTER TABLE login_challenges DROP COLUMN invite_hash;
