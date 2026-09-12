-- Rejects every update and delete on tables whose rows never change once stored.
CREATE FUNCTION misko.reject_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'rows of % cannot be changed or deleted', TG_TABLE_NAME
        USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = TG_TABLE_NAME || '_immutable';
END;
$$;

-- A physical apparatus set up for one paradigm, such as a particular tank.
-- (id, paradigm_key) lets revisions repeat the paradigm, so it cannot change
-- while revisions exist.
CREATE TABLE misko.environments (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    paradigm_key text NOT NULL CHECK (paradigm_key ~ '^[A-Z][A-Z_]{0,39}$'),
    notes        text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT environments_paradigm_scope_key UNIQUE (id, paradigm_key)
);

CREATE UNIQUE INDEX environments_name_key ON misko.environments (lower(name));

-- Measurements validated against a paradigm version. Rows are immutable; a
-- change is a new revision with the next number.
CREATE TABLE misko.environment_revisions (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    environment_id   uuid NOT NULL,
    paradigm_key     text NOT NULL,
    number           integer NOT NULL CHECK (number >= 1),
    paradigm_version integer NOT NULL CHECK (paradigm_version >= 1),
    apparatus        jsonb NOT NULL CHECK (jsonb_typeof(apparatus) = 'object'),
    notes            text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_by       uuid NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT environment_revisions_environment_fkey FOREIGN KEY (environment_id, paradigm_key) REFERENCES misko.environments (id, paradigm_key),
    CONSTRAINT environment_revisions_number_key UNIQUE (environment_id, number),
    CONSTRAINT environment_revisions_paradigm_scope_key UNIQUE (id, paradigm_key, paradigm_version)
);

CREATE TRIGGER environment_revisions_immutable BEFORE UPDATE OR DELETE ON misko.environment_revisions
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
