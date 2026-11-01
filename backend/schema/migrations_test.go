package schema

import (
	"os"
	"strings"
	"testing"
)

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

// TestPublishedMigrationsNeverChange pins every published migration to its
// checksum. A database that applied a file refuses a changed one, so editing a
// published migration would stop the next deploy; change the schema with a new
// migration instead and add its line to testdata/published.txt.
func TestPublishedMigrationsNeverChange(t *testing.T) {
	raw, err := os.ReadFile("testdata/published.txt")
	if err != nil {
		t.Fatal(err)
	}
	pinned := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sum, name, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("malformed line %q", line)
		}
		pinned[name] = sum
	}
	migrations, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations {
		sum, ok := pinned[m.Name]
		if !ok {
			t.Errorf("%s is not in testdata/published.txt; add %q", m.Name, m.Checksum+" "+m.Name)
			continue
		}
		if sum != m.Checksum {
			t.Errorf("%s changed after it was published; restore it and add a new migration instead", m.Name)
		}
		delete(pinned, m.Name)
	}
	for name := range pinned {
		t.Errorf("published migration %s is missing", name)
	}
}
