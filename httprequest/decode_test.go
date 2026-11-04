package httprequest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
)

type (
	namedString string
	namedBool   bool
	namedInt    int
	namedUint   uint
	namedFloat  float64
	decodeDoc   struct {
		A int `json:"a"`
	}
)

func TestProcessResponseDecodesEveryKind(t *testing.T) {
	t.Parallel()

	h := &HTTPRequest{}
	var (
		s    string
		ns   namedString
		b    bool
		nb   namedBool
		i    int
		ni   namedInt
		i8   int8
		u    uint
		nu   namedUint
		f    float64
		nf   namedFloat
		raw  []byte
		doc  decodeDoc
		m    map[string]int
		sl   []int
		anyV any
	)

	tests := []struct {
		name  string
		body  string
		out   any
		check func(t *testing.T)
	}{
		{"string", "hello", &s, func(t *testing.T) { require.Equal(t, "hello", s) }},
		{"named string", "hello", &ns, func(t *testing.T) { require.Equal(t, namedString("hello"), ns) }},
		{"bool", "true", &b, func(t *testing.T) { require.True(t, b) }},
		{"named bool", " true\n", &nb, func(t *testing.T) { require.Equal(t, namedBool(true), nb) }},
		{"int", "-42", &i, func(t *testing.T) { require.Equal(t, -42, i) }},
		{"named int", "42", &ni, func(t *testing.T) { require.Equal(t, namedInt(42), ni) }},
		{"int8", "7", &i8, func(t *testing.T) { require.Equal(t, int8(7), i8) }},
		{"uint", "42", &u, func(t *testing.T) { require.Equal(t, uint(42), u) }},
		{"named uint", "42", &nu, func(t *testing.T) { require.Equal(t, namedUint(42), nu) }},
		{"float", "1.5", &f, func(t *testing.T) { require.InDelta(t, 1.5, f, 1e-9) }},
		{"named float", "1.5", &nf, func(t *testing.T) { require.InDelta(t, 1.5, float64(nf), 1e-9) }},
		{"bytes", "raw", &raw, func(t *testing.T) { require.Equal(t, []byte("raw"), raw) }},
		{"struct", `{"a":1}`, &doc, func(t *testing.T) { require.Equal(t, 1, doc.A) }},
		{"map", `{"a":2}`, &m, func(t *testing.T) { require.Equal(t, 2, m["a"]) }},
		{"slice", `[1,2]`, &sl, func(t *testing.T) { require.Equal(t, []int{1, 2}, sl) }},
		{"interface", `{"a":3}`, &anyV, func(t *testing.T) { require.NotNil(t, anyV) }},
		{"empty json body", ``, &doc, func(t *testing.T) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				require.NoError(t, h.processResponse(strings.NewReader(tt.body), tt.out))
			})
			tt.check(t)
		})
	}
}

func TestProcessResponseRejectsInvalidOut(t *testing.T) {
	t.Parallel()

	h := &HTTPRequest{}
	tests := []struct {
		name string
		out  any
	}{
		{"non pointer", decodeDoc{}},
		{"typed nil struct pointer", (*decodeDoc)(nil)},
		{"typed nil string pointer", (*string)(nil)},
		{"typed nil named bool pointer", (*namedBool)(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var err error
			require.NotPanics(t, func() { err = h.processResponse(strings.NewReader("true"), tt.out) })
			appErr, ok := err.(*errorkit.AppError)
			require.True(t, ok, "want *errorkit.AppError, got %T (%v)", err, err)
			require.Equal(t, errorkit.ERR_SYSTEM_UNEXPECTED, appErr.Code())
		})
	}
}

func TestProcessResponseParseErrors(t *testing.T) {
	t.Parallel()

	h := &HTTPRequest{}
	var (
		i int
		u uint
		f float64
		b namedBool
		d decodeDoc
	)
	for name, out := range map[string]any{"int": &i, "uint": &u, "float": &f, "bool": &b, "json": &d} {
		t.Run(name, func(t *testing.T) {
			err := h.processResponse(strings.NewReader("{not-valid"), out)
			appErr, ok := err.(*errorkit.AppError)
			require.True(t, ok, "want *errorkit.AppError, got %T", err)
			require.Equal(t, errorkit.ERR_SYSTEM_UNEXPECTED, appErr.Code())
		})
	}
}
