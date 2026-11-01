-- The pole and the treadmill are filmed from the side: their calibrations
-- measure on a vertical plane of the apparatus (APPARATUS_SIDE).
ALTER TABLE misko.calibrations DROP CONSTRAINT calibrations_measurement_plane_check;
ALTER TABLE misko.calibrations ADD CONSTRAINT calibrations_measurement_plane_check
    CHECK (measurement_plane IN ('ARENA_FLOOR', 'WATER_SURFACE', 'APPARATUS_TOP', 'APPARATUS_SIDE'));
