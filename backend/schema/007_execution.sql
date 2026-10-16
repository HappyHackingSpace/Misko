-- What actually happened to each animal of a running study: every substance
-- given (disease induction, vehicle and treatment alike) and whether its
-- disease model was confirmed.

-- One actual dose. regimen_item_id names the planned item it fulfils, which
-- must belong to the animal's own group; an unplanned dose has none. A per-kg
-- dose pins the weight it was computed from, measured on the same animal.
CREATE TABLE misko.administrations (
    id                    uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id         uuid NOT NULL,
    allocation_id         uuid NOT NULL,
    group_id              uuid NOT NULL,
    subject_id            uuid NOT NULL,
    regimen_item_id       uuid,
    purpose               text NOT NULL CHECK (purpose IN ('INDUCTION', 'VEHICLE', 'TREATMENT')),
    substance_id          uuid NOT NULL REFERENCES misko.substances (id),
    dose_micro            bigint NOT NULL CHECK (dose_micro > 0),
    unit                  text NOT NULL CHECK (unit IN ('mg/kg', 'ug/kg', 'mL/kg', 'IU/kg', 'mg', 'ug', 'g', 'mL', 'uL', 'IU')),
    route                 text NOT NULL CHECK (route IN ('ORAL', 'INTRAPERITONEAL', 'SUBCUTANEOUS', 'INTRAVENOUS', 'INTRAMUSCULAR', 'INTRANASAL',
                                                         'INTRACEREBROVENTRICULAR', 'TOPICAL', 'INHALATION')),
    weight_measurement_id uuid,
    administered_at       timestamptz NOT NULL,
    notes                 text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    recorded_by           uuid NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT administrations_allocation_fkey FOREIGN KEY (experiment_id, allocation_id) REFERENCES misko.allocations (experiment_id, id),
    CONSTRAINT administrations_group_fkey FOREIGN KEY (allocation_id, group_id) REFERENCES misko.allocations (id, group_id),
    CONSTRAINT administrations_subject_fkey FOREIGN KEY (allocation_id, subject_id) REFERENCES misko.allocations (id, subject_id),
    CONSTRAINT administrations_regimen_fkey FOREIGN KEY (group_id, regimen_item_id) REFERENCES misko.regimen_items (group_id, id),
    CONSTRAINT administrations_weight_fkey FOREIGN KEY (subject_id, weight_measurement_id) REFERENCES misko.weight_measurements (subject_id, id),
    CONSTRAINT administrations_per_kg_weight_check CHECK (unit NOT LIKE '%/kg' OR weight_measurement_id IS NOT NULL)
);

CREATE INDEX administrations_allocation_idx ON misko.administrations (allocation_id, administered_at);
CREATE INDEX administrations_experiment_idx ON misko.administrations (experiment_id, administered_at);
CREATE INDEX administrations_substance_idx ON misko.administrations (substance_id);
CREATE TRIGGER administrations_running BEFORE INSERT ON misko.administrations
    FOR EACH ROW EXECUTE FUNCTION misko.check_experiment_running();
CREATE TRIGGER administrations_immutable BEFORE UPDATE OR DELETE ON misko.administrations
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- Whether an animal's disease model took. The induction itself is an
-- administration with purpose INDUCTION; this row is the assessment.
CREATE TABLE misko.induction_outcomes (
    allocation_id uuid PRIMARY KEY,
    experiment_id uuid NOT NULL,
    status        text NOT NULL CHECK (status IN ('CONFIRMED', 'NOT_CONFIRMED')),
    assessed_at   timestamptz NOT NULL,
    note          text CHECK (char_length(note) BETWEEN 1 AND 2000),
    recorded_by   uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT induction_outcomes_allocation_fkey FOREIGN KEY (experiment_id, allocation_id) REFERENCES misko.allocations (experiment_id, id)
);

CREATE TRIGGER induction_outcomes_running BEFORE INSERT ON misko.induction_outcomes
    FOR EACH ROW EXECUTE FUNCTION misko.check_experiment_running();
CREATE TRIGGER induction_outcomes_immutable BEFORE UPDATE OR DELETE ON misko.induction_outcomes
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
