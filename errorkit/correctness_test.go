package errorkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func batchMeta(code ErrorCode, group ErrorGroup, errType ErrorType) Metadata {
	return Metadata{
		Code:        code,
		Type:        errType,
		Group:       group,
		Category:    "test",
		Description: "batch test " + string(code),
		HTTPStatus:  500,
	}
}

// registerTarget runs the same scenario against the package-global registry
// and an isolated Registry.
type registerTarget struct {
	name     string
	register func(...Metadata) error
	get      func(ErrorCode) (Metadata, bool)
}

func registerTargets() []registerTarget {
	r := NewRegistry()
	return []registerTarget{
		{name: "global", register: RegisterMany, get: GetMetadata},
		{name: "registry", register: r.RegisterMany, get: r.GetMetadata},
	}
}

func TestRegisterManyIsAllOrNothing(t *testing.T) {
	for _, target := range registerTargets() {
		t.Run(target.name+"/invalid entry", func(t *testing.T) {
			good := batchMeta(ErrorCode("ERR_BATCH_GOOD_"+strings.ToUpper(target.name)), GroupUnknown, ErrorTypeInternal)
			bad := batchMeta("ERR_BATCH_BAD", GroupUnknown, ErrorTypeInternal)
			bad.Type = ""

			err := target.register(good, bad)
			if !errors.Is(err, ErrInvalidMetadata) {
				t.Fatalf("RegisterMany(good, bad) = %v, want ErrInvalidMetadata", err)
			}
			if _, ok := target.get(good.Code); ok {
				t.Fatal("valid entry stored although the batch was rejected")
			}
		})
		t.Run(target.name+"/conflict inside the batch", func(t *testing.T) {
			first := batchMeta(ErrorCode("ERR_BATCH_SELF_"+strings.ToUpper(target.name)), GroupUnknown, ErrorTypeInternal)
			second := first
			second.Description = "different"

			err := target.register(first, second)
			if !errors.Is(err, ErrMetadataConflict) {
				t.Fatalf("RegisterMany(first, conflicting) = %v, want ErrMetadataConflict", err)
			}
			if _, ok := target.get(first.Code); ok {
				t.Fatal("entry stored although the batch conflicted with itself")
			}
		})
		t.Run(target.name+"/identical duplicate inside the batch", func(t *testing.T) {
			meta := batchMeta(ErrorCode("ERR_BATCH_DUP_"+strings.ToUpper(target.name)), GroupUnknown, ErrorTypeInternal)
			if err := target.register(meta, meta); err != nil {
				t.Fatalf("RegisterMany(meta, meta) = %v, want nil", err)
			}
		})
	}
}

func TestRegistryMergeIsAllOrNothing(t *testing.T) {
	existing := batchMeta("ERR_MERGE_EXISTING", GroupUnknown, ErrorTypeInternal)
	dst := NewRegistry(existing)

	conflict := existing
	conflict.Description = "changed"
	fresh := batchMeta("ERR_MERGE_FRESH", GroupUnknown, ErrorTypeInternal)
	src := NewRegistry(conflict, fresh)

	if err := dst.Merge(src); !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("Merge = %v, want ErrMetadataConflict", err)
	}
	if _, ok := dst.GetMetadata(fresh.Code); ok {
		t.Fatal("Merge stored an entry although it returned a conflict")
	}
}

func TestCodeListingsAreSorted(t *testing.T) {
	r := NewRegistry(
		batchMeta("ERR_SORT_C", GroupData, ErrorTypeInternal),
		batchMeta("ERR_SORT_A", GroupData, ErrorTypeInternal),
		batchMeta("ERR_SORT_B", GroupData, ErrorTypeExternal),
		batchMeta("ERR_SORT_E", GroupData, ErrorTypeInternal),
		batchMeta("ERR_SORT_D", GroupAuth, ErrorTypeInternal),
	)
	if err := RegisterMany(batchMeta("ERR_SORT_GLOBAL_Z", GroupAuth, ErrorTypeExternal),
		batchMeta("ERR_SORT_GLOBAL_Y", GroupAuth, ErrorTypeExternal)); err != nil {
		t.Fatal(err)
	}

	listings := map[string][]ErrorCode{
		"Registry.GetAllCodes":     r.GetAllCodes(),
		"Registry.GetCodesByGroup": r.GetCodesByGroup(GroupData),
		"Registry.GetCodesByType":  r.GetCodesByType(ErrorTypeInternal),
		"GetAllCodes":              GetAllCodes(),
		"GetCodesByGroup":          GetCodesByGroup(GroupAuth),
		"GetCodesByType":           GetCodesByType(ErrorTypeExternal),
	}
	for name, codes := range listings {
		if len(codes) < 2 {
			t.Errorf("%s returned %d codes, want at least 2", name, len(codes))
		}
		if !slices.IsSorted(codes) {
			t.Errorf("%s = %v, want sorted", name, codes)
		}
	}
	if got, want := r.GetCodesByGroup(GroupData), []ErrorCode{"ERR_SORT_A", "ERR_SORT_B", "ERR_SORT_C", "ERR_SORT_E"}; !slices.Equal(got, want) {
		t.Errorf("Registry.GetCodesByGroup(data) = %v, want %v", got, want)
	}
}

func TestJSONFormatterFallbackIsValidJSON(t *testing.T) {
	appErr := NewError(`ERR_"QUOTE\`).With(WithPayload(make(chan int)))

	out := appErr.PrettyJSON()

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("fallback output is not valid JSON: %v\n%s", err, out)
	}
	if got["code"] != `ERR_"QUOTE\` || got["id"] != appErr.ID || got["error"] != "marshal_failed" {
		t.Errorf("fallback = %v", got)
	}
}

// TestWriteToChoosesColorsFromTheTargetWriter makes stdout a character
// device (/dev/null, which isTerminal reports as a terminal) and checks that
// WriteTo to a buffer still writes no ANSI escape codes.
func TestWriteToChoosesColorsFromTheTargetWriter(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devNull.Close() })
	if !isTerminal(devNull) {
		t.Skipf("%s is not a character device here", os.DevNull)
	}

	stdout := os.Stdout
	os.Stdout = devNull
	t.Cleanup(func() { os.Stdout = stdout })

	appErr := NewError(ERR_INTERNAL)
	var buf bytes.Buffer
	if _, err := appErr.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\033[") {
		t.Fatalf("WriteTo(buffer) wrote ANSI codes because stdout is a terminal:\n%q", buf.String())
	}
	if !strings.Contains(buf.String(), "ERR_INTERNAL") {
		t.Fatalf("WriteTo(buffer) = %q, want the formatted error", buf.String())
	}
}

// withULIDState sets the generator's monotonic state and restores it after
// the test.
func withULIDState(t *testing.T, timestamp uint64, randomness [randomSize]byte) {
	t.Helper()
	monotonicMutex.Lock()
	prevTimestamp, prevRandomness := lastTimestamp, lastRandomness
	lastTimestamp, lastRandomness = timestamp, randomness
	monotonicMutex.Unlock()
	t.Cleanup(func() {
		monotonicMutex.Lock()
		lastTimestamp, lastRandomness = prevTimestamp, prevRandomness
		monotonicMutex.Unlock()
	})
}

// TestULIDNeverGoesBackwards simulates a newer timestamp already stored by
// another goroutine (or a clock step backwards): the next ID must still sort
// after the previous one.
func TestULIDNeverGoesBackwards(t *testing.T) {
	future := uint64(time.Now().Add(time.Hour).UnixMilli())
	cases := []struct {
		name       string
		randomness [randomSize]byte
		wantTime   uint64
	}{
		{name: "increment randomness", randomness: [randomSize]byte{9: 7}, wantTime: future},
		{name: "randomness overflow moves to the next millisecond", randomness: [randomSize]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, wantTime: future + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withULIDState(t, future, tc.randomness)
			previous := encodeULID(future, tc.randomness)

			start := time.Now()
			id := generateULID()
			if time.Since(start) > 500*time.Millisecond {
				t.Fatalf("generateULID blocked for %s", time.Since(start))
			}
			if id <= previous {
				t.Fatalf("generateULID() = %s, want > previous %s", id, previous)
			}
			if got := uint64(ParseULID(id).UnixMilli()); got != tc.wantTime {
				t.Fatalf("timestamp = %d, want %d", got, tc.wantTime)
			}
		})
	}
}

func TestULIDDecodingIsCaseInsensitive(t *testing.T) {
	id := generateULID()
	lower := strings.ToLower(id)

	if !ValidateULID(lower) {
		t.Fatalf("ValidateULID(%q) = false, want true", lower)
	}
	if got, want := ParseULID(lower), ParseULID(id); !got.Equal(want) || got.IsZero() {
		t.Fatalf("ParseULID(%q) = %v, want %v", lower, got, want)
	}
	for _, invalid := range []string{"01HV8F3KJ2XQDT8JQMZR4Y5WNI", "01hv8f3kj2xqdt8jqmzr4y5wnu", "01HV8F3KJ2", ""} {
		if ValidateULID(invalid) {
			t.Errorf("ValidateULID(%q) = true, want false", invalid)
		}
	}
}
