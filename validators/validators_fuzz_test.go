package validators

import (
	"testing"
	"time"
)

// Invariants: the validators never panic, and a nil error implies the
// documented grammar.

func FuzzIsValidDatePointer(f *testing.F) {
	for _, s := range []string{"2024-02-29", "2023-02-29", "2024-1-5", " 2024-01-05", "", "２０２４-01-05"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if IsValidDatePointer(&s) != nil {
			return
		}
		d, err := time.Parse(DateLayout, s)
		if err != nil {
			t.Fatalf("accepted %q, which time.Parse rejects: %v", s, err)
		}
		if got := d.Format(DateLayout); got != s {
			t.Fatalf("accepted %q, which is not canonical YYYY-MM-DD (%q)", s, got)
		}
	})
}

func FuzzIsAlphanumericWithSpaces(f *testing.F) {
	for _, s := range []string{"hello world 123", "  a  ", "", "José", "a\tb", "a b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if IsAlphanumericWithSpaces(s) != nil {
			return
		}
		nonSpace := false
		for _, r := range s {
			if !isAlphanumericOrSpace(r) {
				t.Fatalf("accepted %q with forbidden rune %q", s, r)
			}
			nonSpace = nonSpace || r != ' '
		}
		if !nonSpace {
			t.Fatalf("accepted blank %q", s)
		}
	})
}

func FuzzValidateDateRange(f *testing.F) {
	f.Add("2024-01-01", "2024-12-31")
	f.Add("2024-05-05", "2024-05-05")
	f.Add("2024-02-02", "2024-02-01")
	f.Add("bad", "2024-01-01")
	f.Fuzz(func(t *testing.T, from, to string) {
		if ValidateDateRange(from, to) != nil {
			return
		}
		fd, err1 := time.Parse(DateLayout, from)
		td, err2 := time.Parse(DateLayout, to)
		if err1 != nil || err2 != nil || fd.After(td) {
			t.Fatalf("accepted from=%q to=%q", from, to)
		}
	})
}
