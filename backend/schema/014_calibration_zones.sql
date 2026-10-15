-- Zone shapes drawn on the camera frame with a calibration, for paradigms
-- whose zones are not derived from parameters (EPM arms, Y maze arms, Barnes
-- holes, three-chamber compartments). A shape belongs to its calibration (the
-- default of an environment revision or a per animal override) and is keyed by
-- the paradigm's zone key; a key may have several shapes, numbered by
-- position. Each shape is stored as drawn, in full-frame pixels, and in
-- centimetres converted with the calibration's fitted transform. A circle is
-- {"x", "y", "radius"}; a polygon is an array of {"x", "y"} vertices. Shapes
-- are written with their calibration and never change.
CREATE TABLE misko.calibration_zones (
    calibration_id uuid NOT NULL REFERENCES misko.calibrations (id),
    zone_key       text NOT NULL CHECK (zone_key ~ '^[a-z][a-z0-9_]{0,59}$'),
    position       integer NOT NULL CHECK (position BETWEEN 0 AND 99),
    kind           text NOT NULL CHECK (kind IN ('CIRCLE', 'POLYGON')),
    pixels         jsonb NOT NULL,
    centimeters    jsonb NOT NULL,
    CONSTRAINT calibration_zones_pkey PRIMARY KEY (calibration_id, zone_key, position),
    CONSTRAINT calibration_zones_shape_check CHECK (
        (kind = 'CIRCLE' AND jsonb_typeof(pixels) = 'object' AND jsonb_typeof(centimeters) = 'object'
            AND jsonb_typeof(pixels -> 'x') = 'number' AND jsonb_typeof(pixels -> 'y') = 'number'
            AND jsonb_typeof(pixels -> 'radius') = 'number' AND (pixels ->> 'radius')::double precision > 0
            AND jsonb_typeof(centimeters -> 'x') = 'number' AND jsonb_typeof(centimeters -> 'y') = 'number'
            AND jsonb_typeof(centimeters -> 'radius') = 'number' AND (centimeters ->> 'radius')::double precision > 0)
        OR (kind = 'POLYGON' AND jsonb_typeof(pixels) = 'array' AND jsonb_typeof(centimeters) = 'array'
            AND jsonb_array_length(pixels) BETWEEN 3 AND 100 AND jsonb_array_length(centimeters) = jsonb_array_length(pixels)))
);

CREATE TRIGGER calibration_zones_immutable BEFORE UPDATE OR DELETE ON misko.calibration_zones
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- A shape is added only in the transaction that created its calibration, so a
-- calibration never gains zones later, and it lies inside the camera frame.
CREATE FUNCTION misko.check_calibration_zone() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    width  double precision;
    height double precision;
    fresh  boolean;
    v      jsonb;
BEGIN
    SELECT c.frame_width, c.frame_height, c.xmin = pg_current_xact_id()::xid INTO width, height, fresh
    FROM misko.calibrations c WHERE c.id = NEW.calibration_id;
    IF NOT FOUND THEN
        RETURN NEW;
    END IF;
    IF NOT fresh THEN
        RAISE EXCEPTION 'calibration % cannot gain zones after it was created', NEW.calibration_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'calibration_zones_frozen';
    END IF;
    IF NEW.kind = 'CIRCLE' THEN
        IF (NEW.pixels ->> 'x')::double precision - (NEW.pixels ->> 'radius')::double precision < 0
           OR (NEW.pixels ->> 'y')::double precision - (NEW.pixels ->> 'radius')::double precision < 0
           OR (NEW.pixels ->> 'x')::double precision + (NEW.pixels ->> 'radius')::double precision > width
           OR (NEW.pixels ->> 'y')::double precision + (NEW.pixels ->> 'radius')::double precision > height THEN
            RAISE EXCEPTION 'zone % of calibration % leaves the frame', NEW.zone_key, NEW.calibration_id
                USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'calibration_zones_frame';
        END IF;
    ELSE
        FOR v IN SELECT jsonb_array_elements(NEW.pixels) LOOP
            IF jsonb_typeof(v -> 'x') IS DISTINCT FROM 'number' OR jsonb_typeof(v -> 'y') IS DISTINCT FROM 'number'
               OR (v ->> 'x')::double precision < 0 OR (v ->> 'y')::double precision < 0
               OR (v ->> 'x')::double precision > width OR (v ->> 'y')::double precision > height THEN
                RAISE EXCEPTION 'zone % of calibration % leaves the frame', NEW.zone_key, NEW.calibration_id
                    USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'calibration_zones_frame';
            END IF;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER calibration_zones_scope BEFORE INSERT ON misko.calibration_zones
    FOR EACH ROW EXECUTE FUNCTION misko.check_calibration_zone();
