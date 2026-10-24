package migrations

import (
	"io/fs"
	"slices"
	"testing"
)

// Every migration exists for both databases, so a Solo install and a Datacenter install always
// have the same schema version.
func TestDialectsMatch(t *testing.T) {
	names := func(f fs.FS) []string {
		got, err := fs.Glob(f, "*.sql")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	pg, lite := names(Postgres), names(SQLite)
	if len(pg) == 0 || !slices.Equal(pg, lite) {
		t.Fatalf("postgres migrations %v and sqlite migrations %v must match one to one", pg, lite)
	}
}
