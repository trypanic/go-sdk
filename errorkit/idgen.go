package errorkit

import (
	"crypto/rand"
	"math"
	"sync"
	"time"
)

// ==============================
// ULID Implementation
// ==============================

// ULID (Universally Unique Lexicographically Sortable Identifier)
// - 128-bit identifier
// - Lexicographically sortable
// - Encodes timestamp (48 bits) + randomness (80 bits)
// - Case-insensitive base32 encoding (26 characters)
// - Monotonically increasing within the same millisecond

const (
	// Encoding defines the base32 alphabet used by ULID
	encoding = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	// encodedSize is the length of a ULID string (26 characters)
	encodedSize = 26

	// randomSize is the number of bytes for randomness (10 bytes = 80 bits)
	randomSize = 10
)

var (
	// GenerateID is the function used to generate error IDs
	// Can be replaced with custom implementation if needed
	GenerateID = generateULID

	// monotonic state for same-millisecond IDs
	lastTimestamp  uint64
	lastRandomness [randomSize]byte
	monotonicMutex sync.Mutex
)

// generateULID generates a new ULID string
func generateULID() string {
	// Get current timestamp in milliseconds
	now := uint64(time.Now().UnixMilli())

	monotonicMutex.Lock()
	defer monotonicMutex.Unlock()

	var randomness [randomSize]byte

	if now == lastTimestamp {
		// Same millisecond: increment randomness for monotonicity
		randomness = lastRandomness
		if !incrementRandomness(&randomness) {
			// Overflow: wait for next millisecond
			time.Sleep(time.Millisecond)
			now = uint64(time.Now().UnixMilli())
			if _, err := rand.Read(randomness[:]); err != nil {
				// Fallback to time-based randomness if crypto/rand fails
				generateFallbackRandomness(&randomness)
			}
		}
	} else {
		// New millisecond: generate fresh randomness
		if _, err := rand.Read(randomness[:]); err != nil {
			// Fallback to time-based randomness if crypto/rand fails
			generateFallbackRandomness(&randomness)
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

// generateFallbackRandomness generates pseudo-random bytes when crypto/rand fails
func generateFallbackRandomness(randomness *[randomSize]byte) {
	// Use current nanosecond time as seed
	nano := time.Now().UnixNano()

	for i := 0; i < randomSize; i++ {
		// Simple LCG (Linear Congruential Generator)
		nano = nano*1103515245 + 12345
		randomness[i] = byte((nano >> 16) & 0xFF)
	}
}

// ==============================
// Custom ID Generator Support
// ==============================

// SetIDGenerator allows replacing the default ULID generator
// Example: errorkit.SetIDGenerator(func() string { return uuid.New().String() })
func SetIDGenerator(generator func() string) {
	if generator == nil {
		GenerateID = generateULID
		return
	}
	GenerateID = generator
}

// ==============================
// ULID Utilities
// ==============================

// ParseULID extracts timestamp from a ULID string
// Returns 0 if the ULID is invalid
func ParseULID(ulid string) time.Time {
	if len(ulid) != encodedSize {
		return time.Time{}
	}

	// Decode timestamp from first 10 characters
	var timestamp uint64
	for i := 0; i < 10; i++ {
		c := ulid[i]
		var value byte
		switch {
		case c >= '0' && c <= '9':
			value = c - '0'
		case c >= 'A' && c <= 'Z':
			// Skip I, L, O, U in base32 alphabet
			if c == 'I' || c == 'L' || c == 'O' || c == 'U' {
				return time.Time{}
			}
			// Map A-Z to 10-31 (skipping invalid chars)
			switch c {
			case 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H':
				value = c - 'A' + 10
			case 'J', 'K':
				value = c - 'J' + 18
			case 'M', 'N':
				value = c - 'M' + 20
			case 'P', 'Q', 'R', 'S', 'T':
				value = c - 'P' + 22
			case 'V', 'W', 'X', 'Y', 'Z':
				value = c - 'V' + 27
			default:
				return time.Time{}
			}
		default:
			return time.Time{}
		}

		timestamp = (timestamp << 5) | uint64(value)
	}

	// Timestamp is in milliseconds
	return time.UnixMilli(int64(timestamp))
}

// ValidateULID checks if a string is a valid ULID
func ValidateULID(ulid string) bool {
	if len(ulid) != encodedSize {
		return false
	}

	for i := 0; i < encodedSize; i++ {
		c := ulid[i]
		// Check if character is in encoding alphabet
		valid := false
		for j := 0; j < len(encoding); j++ {
			if c == encoding[j] {
				valid = true
				break
			}
		}
		if !valid {
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
