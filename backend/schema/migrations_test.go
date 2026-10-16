package schema

import "testing"

func TestMigrationsAreNumberedWithoutGaps(t *testing.T) {
	migrations, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) == 0 || Latest() != len(migrations) {
		t.Fatalf("latest %d of %d migrations", Latest(), len(migrations))
	}
	if migrations[0].Name != "001_foundation.sql" {
		t.Fatalf("first migration is %s", migrations[0].Name)
	}
	for _, name := range []string{"1_initial.sql", "001-initial.sql", "001_Initial.sql", "001_initial.SQL"} {
		if fileName.MatchString(name) {
			t.Errorf("%s accepted", name)
		}
	}
}
