//go:build integration

package postgres_test

import (
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/catalog"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"testing"
	"time"
)

func square(x0, y0, x1, y1 float64) domain.Shape {
	return domain.Shape{Polygon: []domain.Vertex{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}}
}

// zonedCatalog is the published catalog, except that the water maze declares
// two required calibrated zones and an optional one. No published paradigm
// declares calibrated zones, but their storage stays generic.
type zonedCatalog struct{ catalog.Catalog }

func (c zonedCatalog) Requirement(key string, version int, parameters map[string]float64) (domain.Requirement, error) {
	req, err := c.Catalog.Requirement(key, version, parameters)
	req.Bounds = nil
	req.Zones = []domain.ZoneRequirement{{Key: "center", Required: true}, {Key: "edges", Required: true}, {Key: "lanes", Required: false}}
	return req, err
}

// Zones are stored with their calibration in pixels and centimetres, read
// back with it, and never change or grow afterwards.
func TestZonesBelongToTheirCalibration(t *testing.T) {
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	s := application.New(store, zonedCatalog{catalog.New()}, time.Now)
	lab := access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.LabManager}
	env := id(t, pool, "INSERT INTO misko.environments (name, paradigm_key) VALUES ('Pool', 'MWM') RETURNING id")
	revision := id(t, pool, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'MWM', 1, 1, '{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}', $2) RETURNING id`, env, lab.UserID)

	in := input("")
	if _, err := s.CalibrateEnvironment(ctx, lab, env, 1, in); !errors.Is(err, domain.ErrMissingZone) {
		t.Fatalf("without the required zones: %v", err)
	}
	in.Zones = []domain.ZoneInput{
		{Key: "lanes", Shape: square(200, 480, 860, 600)},
		{Key: "lanes", Shape: square(1060, 480, 1720, 600)},
		{Key: "edges", Shape: square(900, 100, 1020, 440)},
		{Key: "edges", Shape: square(900, 640, 1020, 980)},
		{Key: "center", Shape: domain.Shape{Circle: &domain.Circle{X: 960, Y: 540, Radius: 60}}},
	}
	created, err := s.CalibrateEnvironment(ctx, lab, env, 1, in)
	if err != nil || len(created.Zones) != 5 {
		t.Fatalf("calibrate with zones: %+v %v", created, err)
	}
	view, err := s.EnvironmentStatus(ctx, viewer, env, 1)
	if err != nil || view.Status != application.Calibrated || len(view.Current.Zones) != 5 {
		t.Fatalf("status: %+v %v", view, err)
	}
	read := view.Current.Zones
	if read[0].Key != "center" || read[0].Pixels.Circle == nil || read[0].Centimeters.Circle.Radius <= 0 ||
		read[1].Key != "edges" || read[1].Pixels.Polygon[0] != (domain.Vertex{X: 900, Y: 100}) ||
		read[3].Key != "lanes" || read[4].Pixels.Polygon[0] != (domain.Vertex{X: 1060, Y: 480}) {
		t.Fatalf("zones read back: %+v", read)
	}
	x, y, _ := truth.Apply(900, 100)
	if v := read[1].Centimeters.Polygon[0]; v.X-x > 1e-6 || x-v.X > 1e-6 || v.Y-y > 1e-6 || y-v.Y > 1e-6 {
		t.Fatalf("centimetres: %+v want %v %v", v, x, y)
	}

	// The database refuses a shape added later, a changed shape and a shape off the frame.
	if _, err := pool.Exec(ctx, `INSERT INTO misko.calibration_zones (calibration_id, zone_key, position, kind, pixels, centimeters)
		VALUES ($1, 'center', 1, 'CIRCLE', '{"x": 10, "y": 10, "radius": 5}', '{"x": 1, "y": 1, "radius": 1}')`, created.ID); !violates(err, "calibration_zones_frozen") {
		t.Fatalf("zone added later: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE misko.calibration_zones SET zone_key = 'other' WHERE calibration_id = $1", created.ID); err == nil {
		t.Fatal("a zone changed")
	}
	bad := created
	bad.ID, bad.SupersedesID = "", created.ID
	bad.Zones = []domain.Zone{{Key: "center", Pixels: domain.Shape{Circle: &domain.Circle{X: 10, Y: 10, Radius: 50}}, Centimeters: domain.Shape{Circle: &domain.Circle{X: 1, Y: 1, Radius: 1}}}}
	if err := store.Transaction(ctx, func(tx application.Store) error { _, err := tx.CreateCalibration(ctx, bad); return err }); !errors.Is(err, domain.ErrZoneOutsideFrame) {
		t.Fatalf("zone off the frame: %v", err)
	}
	bad.Zones[0].Pixels = domain.Shape{Circle: &domain.Circle{X: 100, Y: 100, Radius: -1}}
	if err := store.Transaction(ctx, func(tx application.Store) error { _, err := tx.CreateCalibration(ctx, bad); return err }); !errors.Is(err, domain.ErrInvalidZone) {
		t.Fatalf("negative radius: %v", err)
	}
	if list, err := s.EnvironmentCalibrations(ctx, viewer, env, 1); err != nil || len(list) != 1 || list[0].EnvironmentRevisionID != revision {
		t.Fatalf("calibrations: %+v %v", list, err)
	}
}

func violates(err error, constraint string) bool {
	_, got := pgtx.Violation(err)
	return got == constraint
}
