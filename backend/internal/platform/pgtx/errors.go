package pgtx

import (
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"regexp"
	"strings"
)

var (
	uuidPattern = regexp.MustCompile(`^(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
)

// ValidUUID reports whether id can be compared with a uuid column. Adapters map
// malformed identifiers to not-found instead of sending them to PostgreSQL.
func ValidUUID(id string) bool { return uuidPattern.MatchString(id) }

// EscapeLike escapes wildcards for patterns declared with ESCAPE '\'.
func EscapeLike(s string) string { return likeEscaper.Replace(s) }

// Violation returns the SQLSTATE and constraint name of a PostgreSQL error, or
// empty strings for any other error.
func Violation(err error) (code, constraint string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, pgErr.ConstraintName
	}
	return "", ""
}
