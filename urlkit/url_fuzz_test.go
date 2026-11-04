package urlkit

import (
	"net/url"
	"strings"
	"testing"
)

// Invariants: BuildURL and JoinPath never panic; on success the path cannot
// change the scheme or host of base, and the result has no dot segment.

func FuzzBuildURL(f *testing.F) {
	f.Add("https://api.example.com", "/users", "page", "1")
	f.Add("https://api.example.com/v1/", "users/42", "", "")
	f.Add("https://api.example.com", "//evil.example/x", "a", "b")
	f.Add("https://api.example.com", "/a/%2e%2e/b", "", "")
	f.Add("https://api.example.com?x=1", "/p?q=1#f", "x", "2")
	f.Fuzz(func(t *testing.T, base, path, key, value string) {
		u, err := BuildURL(base, path, map[string]string{key: value})
		if err != nil {
			return
		}
		b, err := url.Parse(base)
		if err != nil {
			t.Fatalf("BuildURL accepted base %q that url.Parse rejects: %v", base, err)
		}
		if u.Scheme != b.Scheme || u.Host != b.Host {
			t.Fatalf("BuildURL(%q, %q) = %q changed scheme/host of base", base, path, u)
		}
		if path != "" && hasDotSegment(u.Path) {
			t.Fatalf("BuildURL(%q, %q) = %q has a dot segment", base, path, u)
		}
	})
}

func FuzzJoinPath(f *testing.F) {
	f.Add("https://api.example.com/", "/v1/", "/users/")
	f.Add("https://api.example.com", "..", "x")
	f.Add("https://api.example.com/base", "a?b", "c#d")
	f.Add("relative/path", "", "x")
	f.Fuzz(func(t *testing.T, base, s1, s2 string) {
		got, err := JoinPath(base, s1, s2)
		if err != nil {
			return
		}
		b, err := url.Parse(base)
		if err != nil {
			t.Fatalf("JoinPath accepted base %q that url.Parse rejects: %v", base, err)
		}
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("JoinPath(%q, %q, %q) = %q, which does not parse: %v", base, s1, s2, got, err)
		}
		if u.Scheme != b.Scheme || u.Host != b.Host {
			t.Fatalf("JoinPath(%q, %q, %q) = %q changed scheme/host of base", base, s1, s2, got)
		}
		for _, s := range []string{s1, s2} {
			if strings.Trim(s, "/") != "" && hasDotSegment(strings.Trim(s, "/")) {
				t.Fatalf("JoinPath accepted dot segment %q", s)
			}
		}
	})
}
