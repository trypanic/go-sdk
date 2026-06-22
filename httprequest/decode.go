package httprequest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"

	"github.com/trypanic/go-sdk/errorkit"
)

// processResponse decodes the response based on the type of 'out'.
func (h *HTTPRequest) processResponse(body io.Reader, out any) error {
	// Get the type of out
	outValue := reflect.ValueOf(out)
	if outValue.Kind() != reflect.Ptr {
		return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
			With(errorkit.WithReason("'out' parameter must be a pointer"))
	}

	outType := outValue.Elem().Type()

	// Handle different entities
	switch outType.Kind() {
	case reflect.String:
		return h.decodeString(body, out)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return h.decodeInt(body, out)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return h.decodeUint(body, out)
	case reflect.Float32, reflect.Float64:
		return h.decodeFloat(body, out)
	case reflect.Bool:
		return h.decodeBool(body, out)
	case reflect.Slice:
		// Check if it's []byte
		if outType == reflect.TypeOf([]byte{}) {
			return h.decodeBytes(body, out)
		}
		// Otherwise decode as JSON
		return h.decodeJSON(body, out)
	case reflect.Struct, reflect.Map, reflect.Interface:
		return h.decodeJSON(body, out)
	default:
		return h.decodeJSON(body, out)
	}
}

// decodeString reads the response body as a string.
func (h *HTTPRequest) decodeString(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}
	*(out.(*string)) = string(data)
	return nil
}

// decodeInt reads the response body and parses it as an integer.
func (h *HTTPRequest) decodeInt(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}

	val, err := strconv.ParseInt(string(bytes.TrimSpace(data)), 10, 64)
	if err != nil {
		return h.buildDecodeError(err)
	}

	outValue := reflect.ValueOf(out).Elem()
	outValue.SetInt(val)
	return nil
}

// decodeUint reads the response body and parses it as an unsigned integer.
func (h *HTTPRequest) decodeUint(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}

	val, err := strconv.ParseUint(string(bytes.TrimSpace(data)), 10, 64)
	if err != nil {
		return h.buildDecodeError(err)
	}

	outValue := reflect.ValueOf(out).Elem()
	outValue.SetUint(val)
	return nil
}

// decodeFloat reads the response body and parses it as a float.
func (h *HTTPRequest) decodeFloat(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}

	val, err := strconv.ParseFloat(string(bytes.TrimSpace(data)), 64)
	if err != nil {
		return h.buildDecodeError(err)
	}

	outValue := reflect.ValueOf(out).Elem()
	outValue.SetFloat(val)
	return nil
}

// decodeBool reads the response body and parses it as a boolean.
func (h *HTTPRequest) decodeBool(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}

	val, err := strconv.ParseBool(string(bytes.TrimSpace(data)))
	if err != nil {
		return h.buildDecodeError(err)
	}

	*(out.(*bool)) = val
	return nil
}

// decodeBytes reads the response body as raw bytes.
func (h *HTTPRequest) decodeBytes(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}
	*(out.(*[]byte)) = data
	return nil
}

// decodeJSON decodes the response body as JSON.
func (h *HTTPRequest) decodeJSON(body io.Reader, out any) error {
	if err := json.NewDecoder(body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			// Empty body is acceptable for some responses (e.g., 204 No Content)
			return nil
		}
		return h.buildDecodeError(err)
	}
	return nil
}

// buildDecodeError creates a standardized decode error.
func (h *HTTPRequest) buildDecodeError(err error) error {
	return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
		With(
			errorkit.WithReason("Failed to decode response body"),
			errorkit.WithWrapped(err),
		)
}
