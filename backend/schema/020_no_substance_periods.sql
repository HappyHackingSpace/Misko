-- No-substance periods: a healthy control receives no substance at all. Its
-- only regimen item is the period it is kept without any substance, purpose
-- NONE, with a first and a last study day and nothing else. Every other
-- purpose still names a substance, a dose, a unit, a route and a frequency.
--
-- A NONE item belongs only to a healthy control, and a healthy control takes
-- only NONE items. Rows written before this migration are left as they are:
-- designs of started experiments are frozen, and a draft's vehicle item in a
-- healthy control is rejected by the start rules until it is changed.
ALTER TABLE misko.regimen_items
    ALTER COLUMN substance_id DROP NOT NULL,
    ALTER COLUMN dose_micro DROP NOT NULL,
    ALTER COLUMN unit DROP NOT NULL,
    ALTER COLUMN route DROP NOT NULL,
    ALTER COLUMN frequency DROP NOT NULL,
    DROP CONSTRAINT regimen_items_purpose_check,
    ADD CONSTRAINT regimen_items_purpose_check CHECK (purpose IN ('NONE', 'INDUCTION', 'VEHICLE', 'TREATMENT')),
    ADD CONSTRAINT regimen_items_substance_check CHECK (
        CASE WHEN purpose = 'NONE'
            THEN substance_id IS NULL AND dose_micro IS NULL AND unit IS NULL AND route IS NULL AND frequency IS NULL AND interval_days IS NULL
            ELSE substance_id IS NOT NULL AND dose_micro IS NOT NULL AND unit IS NOT NULL AND route IS NOT NULL AND frequency IS NOT NULL
        END);

-- A written item is a NONE item exactly when its group is a healthy control.
-- The group is read under a share lock, so its role cannot change meanwhile.
CREATE FUNCTION misko.check_regimen_role() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    group_role text;
BEGIN
    SELECT role INTO group_role FROM misko.experiment_groups WHERE id = NEW.group_id FOR SHARE;
    IF (group_role = 'HEALTHY_CONTROL') IS DISTINCT FROM (NEW.purpose = 'NONE') THEN
        RAISE EXCEPTION 'a % group cannot receive a % regimen item', group_role, NEW.purpose
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'regimen_items_role';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER regimen_items_role AFTER INSERT OR UPDATE ON misko.regimen_items
    FOR EACH ROW EXECUTE FUNCTION misko.check_regimen_role();

-- A group's role changes only when its items stay valid for the new role:
-- NONE items for a healthy control, no NONE item for any other role.
CREATE FUNCTION misko.check_group_regimen_role() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM misko.regimen_items r
               WHERE r.group_id = NEW.id AND (NEW.role = 'HEALTHY_CONTROL') IS DISTINCT FROM (r.purpose = 'NONE')) THEN
        RAISE EXCEPTION 'group % has regimen items a % group cannot receive', NEW.id, NEW.role
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'experiment_groups_regimen_role';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER experiment_groups_regimen_role AFTER UPDATE OF role ON misko.experiment_groups
    FOR EACH ROW WHEN (OLD.role IS DISTINCT FROM NEW.role) EXECUTE FUNCTION misko.check_group_regimen_role();

-- Nothing is given in a no-substance period, so no dose refers to one.
CREATE FUNCTION misko.check_administration_item() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.regimen_item_id IS NOT NULL
       AND EXISTS (SELECT 1 FROM misko.regimen_items WHERE id = NEW.regimen_item_id AND purpose = 'NONE') THEN
        RAISE EXCEPTION 'regimen item % gives no substance and cannot be administered', NEW.regimen_item_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'administrations_no_substance';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER administrations_no_substance AFTER INSERT ON misko.administrations
    FOR EACH ROW EXECUTE FUNCTION misko.check_administration_item();
