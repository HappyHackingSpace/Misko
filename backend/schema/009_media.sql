-- A video object in private storage. The database stores bucket, object name
-- and generation, never signed URLs or upload session URIs. A verified asset
-- never changes, so analysis runs can pair with a fixed original.
CREATE TABLE misko.video_assets (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    kind             text NOT NULL CHECK (kind IN ('ORIGINAL', 'ANALYZED')),
    bucket           text NOT NULL CHECK (bucket ~ '^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$'),
    object_name      text NOT NULL CHECK (char_length(object_name) BETWEEN 1 AND 1024),
    content_type     text NOT NULL CHECK (content_type IN ('video/mp4', 'video/quicktime', 'video/webm')),
    file_name        text CHECK (char_length(file_name) BETWEEN 1 AND 255),
    size_bytes       bigint NOT NULL CHECK (size_bytes > 0),
    crc32c           bigint NOT NULL CHECK (crc32c BETWEEN 0 AND 4294967295),
    status           text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'VERIFIED', 'REJECTED')),
    generation       bigint CHECK (generation > 0),
    rejection_reason text CHECK (rejection_reason IN ('OVERSIZED', 'SIZE_MISMATCH', 'CHECKSUM_MISMATCH', 'CONTENT_TYPE_MISMATCH')),
    verified_at      timestamptz,
    created_by       uuid NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT video_assets_object_key UNIQUE (bucket, object_name),
    CONSTRAINT video_assets_state_check CHECK (
        (status = 'PENDING' AND generation IS NULL AND verified_at IS NULL AND rejection_reason IS NULL)
        OR (status = 'VERIFIED' AND generation IS NOT NULL AND verified_at IS NOT NULL AND rejection_reason IS NULL)
        OR (status = 'REJECTED' AND generation IS NOT NULL AND verified_at IS NULL AND rejection_reason IS NOT NULL))
);

-- Only a pending asset may become verified or rejected; nothing else changes
-- and assets are never deleted.
CREATE FUNCTION misko.check_video_asset_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE'
       OR OLD.status <> 'PENDING'
       OR ROW(NEW.id, NEW.kind, NEW.bucket, NEW.object_name, NEW.content_type, NEW.file_name, NEW.size_bytes, NEW.crc32c, NEW.created_by, NEW.created_at)
          IS DISTINCT FROM
          ROW(OLD.id, OLD.kind, OLD.bucket, OLD.object_name, OLD.content_type, OLD.file_name, OLD.size_bytes, OLD.crc32c, OLD.created_by, OLD.created_at) THEN
        RAISE EXCEPTION 'video asset % cannot change', OLD.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'video_assets_immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER video_assets_change BEFORE UPDATE OR DELETE ON misko.video_assets
    FOR EACH ROW EXECUTE FUNCTION misko.check_video_asset_change();

-- The part of a source video that belongs to a test, in microseconds from the
-- start of the video.
CREATE TABLE misko.test_recordings (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id  uuid NOT NULL,
    test_id        uuid NOT NULL,
    video_asset_id uuid NOT NULL REFERENCES misko.video_assets (id),
    clip_start_us  bigint NOT NULL DEFAULT 0 CHECK (clip_start_us >= 0),
    clip_end_us    bigint,
    created_by     uuid NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT test_recordings_clip_check CHECK (clip_end_us IS NULL OR clip_end_us > clip_start_us),
    CONSTRAINT test_recordings_test_fkey FOREIGN KEY (experiment_id, test_id) REFERENCES misko.tests (experiment_id, id),
    CONSTRAINT test_recordings_asset_key UNIQUE (video_asset_id),
    CONSTRAINT test_recordings_scope_key UNIQUE (test_id, id)
);

CREATE INDEX test_recordings_test_idx ON misko.test_recordings (test_id, created_at, id);

CREATE TRIGGER test_recordings_immutable BEFORE UPDATE OR DELETE ON misko.test_recordings
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
