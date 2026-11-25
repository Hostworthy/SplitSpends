-- +goose Up
-- The schema refuses amounts, currencies and split settings the domain never
-- writes, so a bad write fails instead of changing balances. Currency codes
-- are stored the way money.Lookup returns them; normalize any that are not.
UPDATE users SET default_currency = upper(btrim(default_currency)) WHERE default_currency <> upper(btrim(default_currency));
UPDATE groups SET default_currency = upper(btrim(default_currency)) WHERE default_currency <> upper(btrim(default_currency));
UPDATE expenses SET currency = upper(btrim(currency)) WHERE currency <> upper(btrim(currency));

-- NOT VALID enforces each check on every new or changed row without reading
-- the existing ones.
ALTER TABLE expenses ADD CONSTRAINT expenses_tax_check CHECK (tax >= 0) NOT VALID;
ALTER TABLE expenses ADD CONSTRAINT expenses_tip_check CHECK (tip >= 0) NOT VALID;
ALTER TABLE expenses ADD CONSTRAINT expenses_currency_check CHECK (currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE expenses ADD CONSTRAINT expenses_method_check
    CHECK (method IN ('equal', 'exact', 'percent', 'shares', 'adjustment', 'itemized')) NOT VALID;
ALTER TABLE expenses ADD CONSTRAINT expenses_recurrence_check
    CHECK (recurrence IN ('none', 'weekly', 'biweekly', 'monthly', 'yearly')) NOT VALID;
ALTER TABLE expense_shares ADD CONSTRAINT expense_shares_paid_check CHECK (paid >= 0) NOT VALID;
ALTER TABLE expense_shares ADD CONSTRAINT expense_shares_owed_check CHECK (owed >= 0) NOT VALID;
ALTER TABLE users ADD CONSTRAINT users_default_currency_check CHECK (default_currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE groups ADD CONSTRAINT groups_default_currency_check CHECK (default_currency ~ '^[A-Z]{3}$') NOT VALID;

-- Validate each check on the rows already stored. A check that an existing
-- row fails (a manual edit, say) stays unvalidated with a warning instead of
-- stopping the server from starting; it still applies to every new write.
-- +goose StatementBegin
DO $$
DECLARE
    c record;
BEGIN
    FOR c IN SELECT conrelid::regclass AS tbl, conname FROM pg_constraint
        WHERE conname IN ('expenses_tax_check', 'expenses_tip_check', 'expenses_currency_check',
            'expenses_method_check', 'expenses_recurrence_check', 'expense_shares_paid_check',
            'expense_shares_owed_check', 'users_default_currency_check', 'groups_default_currency_check')
    LOOP
        BEGIN
            EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', c.tbl, c.conname);
        EXCEPTION WHEN check_violation THEN
            RAISE WARNING '% has rows that fail %; fix them and run ALTER TABLE % VALIDATE CONSTRAINT %',
                c.tbl, c.conname, c.tbl, c.conname;
        END;
    END LOOP;
END
$$;
-- +goose StatementEnd
