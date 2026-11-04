package marshal

import (
	"encoding/json"
	"testing"
)

// Invariants: for any string, NoEscapeBytesNoNewline returns valid JSON with
// no trailing newline that decodes back to the same value (invalid UTF-8 is
// replaced with U+FFFD by encoding/json, so the comparison uses the value
// json.Marshal round-trips to).
func FuzzNoEscapeBytesNoNewline(f *testing.F) {
	for _, s := range []string{"", "plain", "<a href=\"x\">&</a>", "line\nbreak", " ", "\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got, err := NoEscapeBytesNoNewline(map[string]string{"k": s})
		if err != nil {
			t.Fatalf("NoEscapeBytesNoNewline(%q): %v", s, err)
		}
		if !json.Valid(got) {
			t.Fatalf("output %q is not valid JSON", got)
		}
		if n := len(got); n > 0 && got[n-1] == '\n' {
			t.Fatalf("output %q has a trailing newline", got)
		}
		std, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var want string
		if err := json.Unmarshal(std, &want); err != nil {
			t.Fatal(err)
		}
		var back map[string]string
		if err := json.Unmarshal(got, &back); err != nil {
			t.Fatalf("output %q does not decode: %v", got, err)
		}
		if back["k"] != want {
			t.Fatalf("round trip = %q, want %q", back["k"], want)
		}
	})
}
