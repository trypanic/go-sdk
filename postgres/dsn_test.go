package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestNormalizePostgresDSN_Table covers C4-007: a raw password containing
// '/' cut the authority short and the DSN failed with "invalid port".
func TestNormalizePostgresDSN_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		dsn      string
		user     string
		password string
		host     string
		database string
	}{
		{"slash in password", "postgres://u:pa/ss@db.internal:5432/app", "u", "pa/ss", "db.internal", "app"},
		{"slash and query", "postgres://u:pa/ss@db.internal:5432/app?sslmode=disable", "u", "pa/ss", "db.internal", "app"},
		{"at signs in password", "postgres://u:p@ss@w@db.internal:5432/app", "u", "p@ss@w", "db.internal", "app"},
		{"question mark and at sign", "postgres://u:p?s@s@db.internal:5432/app", "u", "p?s@s", "db.internal", "app"},
		{"hash and slash", "postgres://u:p#s/s@db.internal:5432/app", "u", "p#s/s", "db.internal", "app"},
		{"no path", "postgres://u:pa/ss@db.internal:5432", "u", "pa/ss", "db.internal", ""},
		{"at sign in query only", "postgres://u:pw@db.internal:5432/app?application_name=a@b", "u", "pw", "db.internal", "app"},
		{"at sign in database without credentials", "postgres://db.internal:5432/app@x", "", "", "db.internal", "app@x"},
		{"at sign in query without credentials", "postgres://db.internal/app?application_name=a@b", "", "", "db.internal", "app"},
		{"ipv6 host", "postgres://u:pa/ss@[::1]:5432/app", "u", "pa/ss", "::1", "app"},
		{"postgresql scheme", "postgresql://u:pa/ss@db.internal/app", "u", "pa/ss", "db.internal", "app"},
		{"already encoded", "postgres://u:pa%2Fss@db.internal:5432/app", "u", "pa/ss", "db.internal", "app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			normalized := normalizePostgresDSN(tt.dsn)
			cfg, err := pgconn.ParseConfig(normalized)
			if err != nil {
				t.Fatalf("normalize(%q) = %q, which does not parse: %v", tt.dsn, normalized, err)
			}
			if tt.user != "" && cfg.User != tt.user {
				t.Fatalf("user = %q, want %q (normalized %q)", cfg.User, tt.user, normalized)
			}
			if tt.user != "" && cfg.Password != tt.password {
				t.Fatalf("password = %q, want %q (normalized %q)", cfg.Password, tt.password, normalized)
			}
			if cfg.Host != tt.host {
				t.Fatalf("host = %q, want %q (normalized %q)", cfg.Host, tt.host, normalized)
			}
			if tt.database != "" && cfg.Database != tt.database {
				t.Fatalf("database = %q, want %q (normalized %q)", cfg.Database, tt.database, normalized)
			}
			if again := normalizePostgresDSN(normalized); again != normalized {
				t.Fatalf("not idempotent: %q -> %q", normalized, again)
			}
		})
	}
}

func TestNormalizePostgresDSN_LeavesOtherFormsUnchanged(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{
		"host=localhost user=u password=p@ss dbname=app",
		"mysql://u:p@ss@host/db",
		"postgres://db.internal:5432/app",
		"postgres://db.internal:5432/app@x",
		"postgres://db.internal/app?application_name=a@b",
		"postgres://[::1]:5432/app@x",
		"postgres://h1:5432,h2:5433/app@x",
	} {
		if got := normalizePostgresDSN(dsn); got != dsn {
			t.Fatalf("normalize(%q) = %q, want unchanged", dsn, got)
		}
	}
}

// FuzzNormalizePostgresDSN checks two properties: normalization is
// idempotent for any input, and a DSN built from raw components inside the
// documented domain (see README "DSN Normalization") round-trips through
// pgconn.ParseConfig to the same user, password, host, port and postgres.
func FuzzNormalizePostgresDSN(f *testing.F) {
	for _, seed := range [][2]string{
		{"u", "pass"},
		{"pguser", "pw@example@1706"},
		{"u", "pa/ss"},
		{"u", "p?s@s"},
		{"u", "p#s/s"},
		{"u", "p:s:s"},
		{"u", "a b"},
		{"svc", ""},
	} {
		f.Add(seed[0], seed[1], "app")
	}

	f.Fuzz(func(t *testing.T, user, password, database string) {
		raw := "postgres://" + user + ":" + password + "@db.internal:5432/" + database

		normalized := normalizePostgresDSN(raw)
		if again := normalizePostgresDSN(normalized); again != normalized {
			t.Fatalf("not idempotent: %q -> %q -> %q", raw, normalized, again)
		}

		if !inDocumentedDomain(user, password, database) {
			return
		}
		cfg, err := pgconn.ParseConfig(normalized)
		if err != nil {
			t.Fatalf("normalize(%q) = %q, which does not parse: %v", raw, normalized, err)
		}
		if cfg.User != user || cfg.Password != password || cfg.Host != "db.internal" || cfg.Port != 5432 || cfg.Database != database {
			t.Fatalf("round trip of %q via %q = user %q password %q host %q port %d database %q",
				raw, normalized, cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
		}
	})
}

// inDocumentedDomain mirrors the limits in the README: printable ASCII; a
// user without ":@/?#%[]"; a password without '%' in which no '@' is followed
// by '/', '?' or '#', and whose text before its first '/', '?' or '#' does
// not look like a port; a plain database name.
func inDocumentedDomain(user, password, database string) bool {
	if user == "" || database == "" {
		return false
	}
	if !printableASCII(user) || !printableASCII(password) {
		return false
	}
	if strings.ContainsAny(user, ":@/?#%[]") || strings.Contains(password, "%") {
		return false
	}
	for _, r := range database {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	if at := strings.IndexByte(password, '@'); at >= 0 && strings.ContainsAny(password[at:], "/?#") {
		return false
	}
	if cut := strings.IndexAny(password, "/?#"); cut >= 0 && !strings.Contains(password[:cut], "@") {
		if looksLikePort(user + ":" + password[:cut]) {
			return false
		}
	}
	return true
}

func printableASCII(s string) bool {
	for i := range len(s) {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}
