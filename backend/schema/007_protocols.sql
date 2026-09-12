CREATE TABLE misko.protocols (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL REFERENCES misko.experiments (id),
    name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description   text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT protocols_scope_key UNIQUE (experiment_id, id)
);

CREATE UNIQUE INDEX protocols_name_key ON misko.protocols (experiment_id, lower(name));

-- Versions and steps are immutable. (experiment_id, id) lets tests reference a
-- version only within its own experiment.
CREATE TABLE misko.protocol_versions (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL,
    protocol_id   uuid NOT NULL,
    number        integer NOT NULL CHECK (number >= 1),
    step_count    integer NOT NULL CHECK (step_count BETWEEN 1 AND 50),
    notes         text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_by    uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT protocol_versions_protocol_fkey FOREIGN KEY (experiment_id, protocol_id) REFERENCES misko.protocols (experiment_id, id),
    CONSTRAINT protocol_versions_number_key UNIQUE (protocol_id, number),
    CONSTRAINT protocol_versions_scope_key UNIQUE (experiment_id, id)
);

-- The composite foreign key keeps a step's paradigm and version equal to those
-- of its environment revision.
CREATE TABLE misko.protocol_steps (
    protocol_version_id     uuid NOT NULL REFERENCES misko.protocol_versions (id),
    position                integer NOT NULL CHECK (position BETWEEN 1 AND 50),
    paradigm_key            text NOT NULL,
    paradigm_version        integer NOT NULL,
    environment_revision_id uuid NOT NULL,
    trial_type              text NOT NULL CHECK (trial_type ~ '^[A-Z][A-Z_]{0,39}$'),
    trials                  integer NOT NULL CHECK (trials BETWEEN 1 AND 1000),
    inter_trial_interval_s  integer NOT NULL CHECK (inter_trial_interval_s BETWEEN 0 AND 86400),
    session                 jsonb NOT NULL CHECK (jsonb_typeof(session) = 'object'),
    notes                   text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    CONSTRAINT protocol_steps_position_key PRIMARY KEY (protocol_version_id, position),
    CONSTRAINT protocol_steps_environment_fkey FOREIGN KEY (environment_revision_id, paradigm_key, paradigm_version)
        REFERENCES misko.environment_revisions (id, paradigm_key, paradigm_version)
);

CREATE INDEX protocol_steps_environment_idx ON misko.protocol_steps (environment_revision_id);

CREATE TRIGGER protocol_versions_immutable BEFORE UPDATE OR DELETE ON misko.protocol_versions
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

CREATE TRIGGER protocol_steps_immutable BEFORE UPDATE OR DELETE ON misko.protocol_steps
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- At commit, a version must have exactly the positions 1 to step_count. Steps
-- added to a version stored by an earlier transaction therefore fail.
CREATE FUNCTION misko.check_protocol_steps() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    version_id uuid;
BEGIN
    IF TG_TABLE_NAME = 'protocol_versions' THEN
        version_id := NEW.id;
    ELSE
        version_id := NEW.protocol_version_id;
    END IF;
    IF (SELECT count(*) FROM misko.protocol_steps s WHERE s.protocol_version_id = version_id)
           IS DISTINCT FROM (SELECT v.step_count FROM misko.protocol_versions v WHERE v.id = version_id)
       OR EXISTS (SELECT 1 FROM misko.protocol_steps s JOIN misko.protocol_versions v ON v.id = s.protocol_version_id
                  WHERE s.protocol_version_id = version_id AND s.position > v.step_count) THEN
        RAISE EXCEPTION 'protocol version % must have steps 1 to its step count', version_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'protocol_steps_complete';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER protocol_versions_steps_complete AFTER INSERT ON misko.protocol_versions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION misko.check_protocol_steps();

CREATE CONSTRAINT TRIGGER protocol_steps_complete AFTER INSERT ON misko.protocol_steps
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION misko.check_protocol_steps();
