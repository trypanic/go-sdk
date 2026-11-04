package errorkit

import (
	"crypto/rand"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ==============================
// ULID Implementation
// ==============================

// ULID (Universally Unique Lexicographically Sortable Identifier)
// - 128-bit identifier
// - Lexicographically sortable
// - Encodes timestamp (48 bits) + randomness (80 bits)
// - Crockford base32 encoding (26 characters); generated IDs are upper
//   case, ParseULID and ValidateULID accept either case
// - Monotonically increasing: every ID sorts after the previous one, also
//   within the same millisecond, under contention and if the clock steps
//   backwards

const (
	// Encoding defines the base32 alphabet used by ULID
	encoding = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	// encodedSize is the length of a ULID string (26 characters)
	encodedSize = 26

	// randomSize is the number of bytes for randomness (10 bytes = 80 bits)
	randomSize = 10
)

var (
	// idGenerator holds a custom ID generator installed by SetIDGenerator;
	// nil means the default ULID generator. Atomic because NewError reads it
	// concurrently with SetIDGenerator.
	idGenerator atomic.Pointer[func() string]

	// monotonic state for same-millisecond IDs
	lastTimestamp  uint64
	lastRandomness [randomSize]byte
	monotonicMutex sync.Mutex
)

// generateULID generates a new ULID string. The clock is read under the
// lock and never moves the stored timestamp backwards, so IDs are strictly
// increasing in the order they are generated.
func generateULID() string {
	monotonicMutex.Lock()
	defer monotonicMutex.Unlock()

	now := uint64(time.Now().UnixMilli())
	var randomness [randomSize]byte

	switch {
	case now > lastTimestamp:
		// New millisecond: fresh randomness. crypto/rand.Read never
		// returns an error (Go 1.24+).
		_, _ = rand.Read(randomness[:])
	default:
		// Same millisecond, or the clock is behind the last ID: keep the
		// last timestamp and increment its randomness.
		now = lastTimestamp
		randomness = lastRandomness
		if !incrementRandomness(&randomness) {
			// 80-bit overflow: move to the next millisecond instead of
			// sleeping under the lock.
			now++
			_, _ = rand.Read(randomness[:])
		}
	}

	lastTimestamp = now
	lastRandomness = randomness

	return encodeULID(now, randomness)
}

// encodeULID encodes timestamp and randomness into ULID string
func encodeULID(timestamp uint64, randomness [randomSize]byte) string {
	// Pre-allocate result buffer
	result := make([]byte, encodedSize)

	// Encode timestamp (10 characters)
	encodeTimestamp(result[:10], timestamp)

	// Encode randomness (16 characters)
	encodeRandomness(result[10:], randomness)

	return string(result)
}

// encodeTimestamp encodes 48-bit timestamp into 10 base32 characters
func encodeTimestamp(dst []byte, timestamp uint64) {
	dst[0] = encoding[(timestamp>>45)&0x1F]
	dst[1] = encoding[(timestamp>>40)&0x1F]
	dst[2] = encoding[(timestamp>>35)&0x1F]
	dst[3] = encoding[(timestamp>>30)&0x1F]
	dst[4] = encoding[(timestamp>>25)&0x1F]
	dst[5] = encoding[(timestamp>>20)&0x1F]
	dst[6] = encoding[(timestamp>>15)&0x1F]
	dst[7] = encoding[(timestamp>>10)&0x1F]
	dst[8] = encoding[(timestamp>>5)&0x1F]
	dst[9] = encoding[timestamp&0x1F]
}

// encodeRandomness encodes 80-bit randomness into 16 base32 characters
func encodeRandomness(dst []byte, randomness [randomSize]byte) {
	// Pack 10 bytes (80 bits) into 16 base32 characters (5 bits each)
	dst[0] = encoding[(randomness[0]>>3)&0x1F]
	dst[1] = encoding[((randomness[0]<<2)|(randomness[1]>>6))&0x1F]
	dst[2] = encoding[(randomness[1]>>1)&0x1F]
	dst[3] = encoding[((randomness[1]<<4)|(randomness[2]>>4))&0x1F]
	dst[4] = encoding[((randomness[2]<<1)|(randomness[3]>>7))&0x1F]
	dst[5] = encoding[(randomness[3]>>2)&0x1F]
	dst[6] = encoding[((randomness[3]<<3)|(randomness[4]>>5))&0x1F]
	dst[7] = encoding[randomness[4]&0x1F]
	dst[8] = encoding[(randomness[5]>>3)&0x1F]
	dst[9] = encoding[((randomness[5]<<2)|(randomness[6]>>6))&0x1F]
	dst[10] = encoding[(randomness[6]>>1)&0x1F]
	dst[11] = encoding[((randomness[6]<<4)|(randomness[7]>>4))&0x1F]
	dst[12] = encoding[((randomness[7]<<1)|(randomness[8]>>7))&0x1F]
	dst[13] = encoding[(randomness[8]>>2)&0x1F]
	dst[14] = encoding[((randomness[8]<<3)|(randomness[9]>>5))&0x1F]
	dst[15] = encoding[randomness[9]&0x1F]
}

// incrementRandomness increments the randomness bytes for monotonicity
// Returns false if overflow occurs
func incrementRandomness(randomness *[randomSize]byte) bool {
	// Increment from least significant byte
	for i := randomSize - 1; i >= 0; i-- {
		randomness[i]++
		if randomness[i] != 0 {
			return true
		}
	}
	// Overflow occurred
	return false
}

// ==============================
// Custom ID Generator Support
// ==============================

// SetIDGenerator allows replacing the default ULID generator. Passing nil
// restores the default. Safe to call concurrently with NewError.
// Example: errorkit.SetIDGenerator(func() string { return uuid.New().String() })
func SetIDGenerator(generator func() string) {
	if generator == nil {
		idGenerator.Store(nil)
		return
	}
	idGenerator.Store(&generator)
}

// generateID returns a new error ID from the configured generator.
func generateID() string {
	if generator := idGenerator.Load(); generator != nil {
		return (*generator)()
	}
	return generateULID()
}

// ==============================
// ULID Utilities
// ==============================

// invalidSymbol marks bytes outside the ULID alphabet in decodeTable.
const invalidSymbol = 0xFF

// decodeTable maps each byte to its 5-bit value in the ULID alphabet, upper
// or lower case, or to invalidSymbol.
var decodeTable = func() (table [256]byte) {
	for i := range table {
		table[i] = invalidSymbol
	}
	for value, c := range []byte(encoding) {
		table[c] = byte(value)
		if 'A' <= c && c <= 'Z' {
			table[c+('a'-'A')] = byte(value)
		}
	}
	return table
}()

// ParseULID extracts the timestamp from a ULID string (either case).
// Returns the zero time if the ULID is invalid.
func ParseULID(ulid string) time.Time {
	if len(ulid) != encodedSize {
		return time.Time{}
	}

	// Decode timestamp from first 10 characters
	var timestamp uint64
	for i := range 10 {
		value := decodeTable[ulid[i]]
		if value == invalidSymbol {
			return time.Time{}
		}
		timestamp = (timestamp << 5) | uint64(value)
	}

	// Timestamp is in milliseconds
	return time.UnixMilli(int64(timestamp))
}

// ValidateULID checks if a string is a valid ULID (either case).
func ValidateULID(ulid string) bool {
	if len(ulid) != encodedSize {
		return false
	}

	for i := range encodedSize {
		if decodeTable[ulid[i]] == invalidSymbol {
			return false
		}
	}

	// Check timestamp is not in the future or too far in the past
	timestamp := ParseULID(ulid)
	if timestamp.IsZero() {
		return false
	}

	now := time.Now()
	// Reject timestamps more than 1 year in the future
	if timestamp.After(now.Add(365 * 24 * time.Hour)) {
		return false
	}

	// Reject timestamps before year 2000
	if timestamp.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return false
	}

	return true
}

// MaxULID returns the maximum ULID for a given timestamp
// Useful for range queries
func MaxULID(t time.Time) string {
	timestamp := uint64(t.UnixMilli())
	var randomness [randomSize]byte
	for i := range randomness {
		randomness[i] = math.MaxUint8
	}
	return encodeULID(timestamp, randomness)
}

// MinULID returns the minimum ULID for a given timestamp
// Useful for range queries
func MinULID(t time.Time) string {
	timestamp := uint64(t.UnixMilli())
	var randomness [randomSize]byte
	// All zeros
	return encodeULID(timestamp, randomness)
}
