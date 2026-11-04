// Package validators provides small input validators with the
// func(any) error shape used by validation libraries such as
// ozzo-validation: dates in YYYY-MM-DD form, date ranges, ASCII
// alphanumeric text and integer ranges. Every failure is an
// *errorkit.AppError; see the error-code policy in validators.go.
package validators
