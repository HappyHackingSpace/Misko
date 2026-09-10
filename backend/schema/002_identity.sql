-- Laboratory singleton and users. Constraints repeat the domain rules so data
-- stays valid even if application validation is bypassed.
CREATE TABLE misko.laboratory (
    singleton  boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    code       text CHECK (code ~ '^[A-Za-z0-9._-]{1,32}$'),
    timezone   text NOT NULL CHECK (timezone <> '' AND timezone <> 'Local'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Roles mirror internal/access/domain. session_version increments on password
-- changes so earlier tokens stop authenticating.
CREATE TABLE misko.users (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    email           text NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254),
    name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    role            text NOT NULL CHECK (role IN ('SUPERADMIN', 'LAB_MANAGER', 'RESEARCHER', 'TECHNICIAN', 'VIEWER')),
    password_hash   text NOT NULL CHECK (password_hash <> ''),
    session_version integer NOT NULL DEFAULT 1 CHECK (session_version > 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_key UNIQUE (email)
);
