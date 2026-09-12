-- Subjects exist independently of experiments. Codes are unique ignoring case.
CREATE TABLE misko.subjects (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    code       text NOT NULL CHECK (code ~ '^[A-Za-z0-9._/-]{1,64}$'),
    species    text NOT NULL CHECK (species IN ('MOUSE', 'RAT')),
    sex        text NOT NULL CHECK (sex IN ('FEMALE', 'MALE', 'UNKNOWN')),
    strain     text CHECK (char_length(strain) BETWEEN 1 AND 120),
    birth_date date,
    notes      text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX subjects_code_key ON misko.subjects (lower(code));
