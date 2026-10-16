-- Laboratory-wide reference data reused across studies. Paradigms are not
-- here: they are versioned templates defined in code, and rows elsewhere refer
-- to them by key and version.

-- A substance is anything given to an animal: an active compound, a reference
-- drug, a vehicle or an agent that induces a disease model. kind only guides
-- pickers; the role of a substance in a study comes from where it is used.
CREATE TABLE misko.substances (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    kind        text NOT NULL CHECK (kind IN ('COMPOUND', 'REFERENCE_DRUG', 'VEHICLE', 'INDUCER')),
    description text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX substances_name_key ON misko.substances (lower(name));
CREATE TRIGGER substances_touch BEFORE UPDATE ON misko.substances FOR EACH ROW EXECUTE FUNCTION misko.touch();

-- How a deficit is produced and how it is confirmed. Every model is induced;
-- the inducing agent and its dose are part of each study's regimen.
CREATE TABLE misko.disease_models (
    id                     uuid PRIMARY KEY DEFAULT uuidv7(),
    name                   text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    induction_type         text NOT NULL CHECK (induction_type IN ('CHEMICAL', 'SURGICAL', 'DIET', 'STRESS', 'OTHER')),
    confirmation_criterion text CHECK (char_length(confirmation_criterion) BETWEEN 1 AND 2000),
    description            text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX disease_models_name_key ON misko.disease_models (lower(name));
CREATE TRIGGER disease_models_touch BEFORE UPDATE ON misko.disease_models FOR EACH ROW EXECUTE FUNCTION misko.touch();
