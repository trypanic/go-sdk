// Package marshal provides JSON encoding helpers that disable HTML escaping,
// so '<', '>' and '&' are written as-is instead of <, > and
// &. It has no decoding helpers; use encoding/json for decoding.
package marshal
