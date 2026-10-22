-- The application namespace and helpers shared by every context.
-- No IF NOT EXISTS: never silently treat an existing installation as fresh.
CREATE SCHEMA misko;
COMMENT ON SCHEMA misko IS 'Misko';

-- Rejects every update and delete on tables whose rows never change once stored.
CREATE FUNCTION misko.reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'rows of % cannot be changed or deleted', TG_TABLE_NAME
        USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = TG_TABLE_NAME || '_immutable';
END;
$$;

-- Keeps updated_at current on tables that allow edits.
CREATE FUNCTION misko.touch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;
