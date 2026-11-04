# validators

Reusable field-level validation functions compatible with the `go-ozzo/ozzo-validation` `validation.By` adapter, covering date formats, alphanumeric strings, integer ranges, and date-range ordering.

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `IsValidDatePointer` | Function | Validate that a `*string` pointer holds a date in `YYYY-MM-DD` format |
| `IsAlphanumericWithSpaces` | Function | Validate that a `string` is non-empty and contains only letters, digits, and spaces |
| `IsWithinRange` | Function | Return a validator that checks an `int` falls within `[lo, hi]` |
| `ValidateDateRange` | Function | Verify that a `from` date string is not after a `to` date string |

---

## Quick Start

```go
import (
    "github.com/trypanic/go-sdk/validators"
    validation "github.com/go-ozzo/ozzo-validation/v4"
)

type Form struct {
    Name     string
    Quantity int
    From     *string
    To       *string
}

func (f *Form) Validate() error {
    return validation.ValidateStruct(f,
        validation.Field(&f.Name, validation.Required, validation.By(validators.IsAlphanumericWithSpaces)),
        validation.Field(&f.Quantity, validation.Required, validation.By(validators.IsWithinRange(1, 65535))),
        validation.Field(f.From, validation.Required, validation.By(validators.IsValidDatePointer)),
        validation.Field(f.To, validation.Required, validation.By(validators.IsValidDatePointer)),
    )
}
```

---

## Error-code policy

Every validator uses the same codes: a value of the wrong type is
`ERR_VALIDATION_INVALID_FORMAT`, an absent value (`nil`, nil pointer, blank
string) is `ERR_VALIDATION_MISSING_FIELD`, and a well-formed value that
violates a constraint is `ERR_VALIDATION_INCONSISTENT`. Assert on `.Code()`,
not on the reason text.

---

## Configuration / Settings

This package has no configuration struct or environment variables.

The date format expected by `IsValidDatePointer` and `ValidateDateRange` is the SDK-owned `DateLayout` constant:

```
YYYY-MM-DD   (Go reference time: "2006-01-02")
```

---

## API Reference

### `IsValidDatePointer`

```go
func IsValidDatePointer(value any) error
```

Designed for use with `validation.By`. Accepts a `*string` and verifies that the pointed-to value parses as a date in `YYYY-MM-DD` format.

**Behaviour:**
- Returns `ERR_VALIDATION_MISSING_FIELD` for `nil` or a nil `*string`, and `ERR_VALIDATION_INVALID_FORMAT` for any other type.
- Parses the dereferenced string with `time.Parse(DateLayout, ...)`.

**Error conditions:**

| Condition | errorkit code |
|---|---|
| `value` is `nil` or a nil `*string` | `ERR_VALIDATION_MISSING_FIELD` — reason: `"value is required"` |
| `value` is not a `*string` (a plain `string` included) | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"value must be a *string"` |
| String does not match `YYYY-MM-DD` | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"must be in format YYYY-MM-DD"` |

---

### `IsAlphanumericWithSpaces`

```go
func IsAlphanumericWithSpaces(value any) error
```

Designed for use with `validation.By`. Accepts a `string` and verifies it is non-empty and contains only ASCII letters (`a-z`, `A-Z`), ASCII digits (`0-9`), and space characters.

**Behaviour:**
- Type-asserts `value` to `string`. Returns an error if the assertion fails.
- Rejects strings that are blank or consist entirely of whitespace.
- Iterates over every rune; any character outside the permitted set causes an immediate error.

**Error conditions:**

| Condition | errorkit code |
|---|---|
| `value` is not a `string` | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"value must be a string"` |
| String is empty or whitespace-only | `ERR_VALIDATION_MISSING_FIELD` — reason: `"cannot be empty or only whitespace"` |
| String contains a non-permitted character | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"must contain only alphanumeric characters and spaces"` |

---

### `IsWithinRange`

```go
func IsWithinRange(lo, hi int) func(any) error
```

Returns a closure suitable for `validation.By`. The closure accepts an `int` value and verifies it falls within the inclusive range `[lo, hi]`.

**Behaviour:**
- If `lo > hi` the bounds are a programming error: the closure always returns `ERR_SYSTEM_CONFIG_INVALID`.
- Type-asserts `value` to `int` (other integer types such as `int64` or `uint` are rejected). Returns an error if the assertion fails.
- Checks `num < lo || num > hi`.

**Error conditions:**

| Condition | errorkit code |
|---|---|
| `lo > hi` | `ERR_SYSTEM_CONFIG_INVALID` — reason: `"invalid range: lower bound <lo> is greater than upper bound <hi>"` |
| `value` is not an `int` | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"value must be an int"` |
| `value` is outside `[lo, hi]` | `ERR_VALIDATION_INCONSISTENT` — reason: `"must be between <lo> and <hi>"` |

---

### `ValidateDateRange`

```go
func ValidateDateRange(from, to string) error
```

Parses both `from` and `to` as `YYYY-MM-DD` date strings and verifies that `from` is not after `to`.

**Behaviour:**
- Calls `time.Parse(DateLayout, from)` and `time.Parse(DateLayout, to)`.
- Returns `ERR_VALIDATION_INVALID_FORMAT` wrapping the `time.Parse` error if either string is malformed.
- If `fromDate.After(toDate)`, returns an errorkit error.

**Edge cases:**
- `from == to` is considered valid (same day is not "after").

**Error conditions:**

| Condition | errorkit code |
|---|---|
| `from` or `to` cannot be parsed | `ERR_VALIDATION_INVALID_FORMAT` |
| `from` is after `to` | `ERR_VALIDATION_INCONSISTENT` — reason: `"from date must not be after to date"` |

---

## Real-World Usage

### Product ingestion — request body validation

A typical ingestion caller uses all four validators to validate the API request body for a product import job.

```go
func (r *RequestBody) validateCommonFields() error {
    return validation.ValidateStruct(r,
        validation.Field(&r.CollectionName, validation.Required,
            validation.By(validators.IsAlphanumericWithSpaces)),
        validation.Field(&r.ProductQuantity, validation.Required,
            validation.By(validators.IsWithinRange(1, 65535))),
        validation.Field(&r.CreatedBy, validation.Required,
            validation.By(validators.IsAlphanumericWithSpaces)),
    )
}

func (r *RequestBody) validateFilterLogic() error {
    // ...
    if hasFilterFrom && hasFilterTo {
        return validators.ValidateDateRange(*r.FilterFrom, *r.FilterTo)
    }
    return nil
}

func (r *RequestBody) validateFilterMode() error {
    return validation.ValidateStruct(r,
        validation.Field(r.FilterFrom, validation.Required,
            validation.By(validators.IsValidDatePointer)),
        validation.Field(r.FilterTo, validation.Required,
            validation.By(validators.IsValidDatePointer)),
    )
}
```

---

## Lifecycle / Concurrency Notes

All functions are pure and stateless. The returned closure from `IsWithinRange` captures only the immutable `lo` and `hi` values passed at construction time. All functions are safe for concurrent use.

---

## Dependencies

| Package | Role |
|---|---|
| `strings` | Whitespace trimming in `IsAlphanumericWithSpaces` |
| `time` | Date parsing in `IsValidDatePointer` and `ValidateDateRange` |
| `github.com/trypanic/go-sdk/errorkit` | Structured error wrapping |

Validation rejections use `errorkit.NewWithoutStack`: metadata, reasons, causes
and error IDs remain available, but `Trace` is empty. Invalid validator
configuration retains a stack. This avoids stack-walking overhead on routine
input rejection; log correlation still uses the error ID and explicit context.
