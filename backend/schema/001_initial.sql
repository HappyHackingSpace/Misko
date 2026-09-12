-- Fresh-install namespace. Domain tables arrive with their owning use cases.
-- No IF NOT EXISTS: never silently treat an existing installation as fresh.
CREATE SCHEMA misko;
COMMENT ON SCHEMA misko IS 'Misko Go backend';
