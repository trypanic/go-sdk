package validators

import "testing"

func BenchmarkValidationFailure(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = IsAlphanumericWithSpaces("invalid!")
	}
}
func BenchmarkValidationSuccess(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = IsAlphanumericWithSpaces("valid")
	}
}
