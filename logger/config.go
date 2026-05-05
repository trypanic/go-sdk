package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/rs/zerolog"
)

const (
	Blue  = "\033[34m"
	Reset = "\033[0m"

	maskedPasswordValue = "*****"
)

// Environment represents the runtime environment.
type Environment string

const (
	Dev  Environment = "development"
	Prod Environment = "production"
)

// DetectEnv reads APP_ENV. Defaults to Dev if unset or unrecognised.
func DetectEnv() Environment {
	switch os.Getenv("ENVIRONMENT") {
	case "production", "prod":
		return Prod
	default:
		return Dev
	}
}

// newWriter returns the io.Writer used by the logger for the given environment.
// Extracted so that logger.Init can compose it with additional writers
// (e.g. OTLPWriter) via zerolog.MultiLevelWriter before building the logger.
func newWriter(env Environment, appName, version string) io.Writer {
	if env == Dev {
		return zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
			NoColor:    false,
			FormatFieldName: func(i any) string {
				return ""
			},
			FormatFieldValue: func(i any) string {
				return ""
			},
			FormatExtra: func(evt map[string]any, buf *bytes.Buffer) error {
				return formatConsoleExtras(evt, buf, appName, version, os.Getenv("ENVIRONMENT") != "local")
			},
		}
	}
	return os.Stdout
}

type consoleField struct {
	key   string
	value any
}

func formatConsoleExtras(
	evt map[string]any,
	buf *bytes.Buffer,
	appName, version string,
	includeServiceMetadata bool,
) error {
	trimTrailingSpaces(buf)

	inlineFields := make([]consoleField, 0, len(evt)+2)

	if includeServiceMetadata {
		inlineFields = append(inlineFields,
			consoleField{key: "service_name", value: appName},
			consoleField{key: "service_version", value: version},
		)
	}

	for _, key := range orderedConsoleExtraKeys(evt) {
		if shouldSkipConsoleExtraField(key) {
			continue
		}

		inlineFields = append(inlineFields, consoleField{key: key, value: evt[key]})
	}

	if len(inlineFields) > 0 {
		if buf.Len() > 0 {
			buf.WriteString(" | ")
		}
		for i, field := range inlineFields {
			if i > 0 {
				buf.WriteByte(' ')
			}
			writeConsoleInlineField(buf, field)
		}
	}

	return nil
}

func orderedConsoleExtraKeys(evt map[string]any) []string {
	keys := make([]string, 0, len(evt))
	for key := range evt {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func shouldSkipConsoleExtraField(key string) bool {
	switch key {
	case zerolog.LevelFieldName,
		zerolog.CallerFieldName,
		zerolog.TimestampFieldName,
		zerolog.MessageFieldName:
		return true
	default:
		return false
	}
}

func writeConsoleInlineField(buf *bytes.Buffer, field consoleField) {
	buf.WriteString(colorizeConsoleFieldName(field.key))
	buf.WriteByte('=')
	buf.WriteString(renderConsoleInlineValue(field.key, field.value))
}

func colorizeConsoleFieldName(key string) string {
	return fmt.Sprintf("%s%s%s", Blue, key, Reset)
}

func renderConsoleInlineValue(fieldKey string, value any) string {
	switch v := value.(type) {
	case string:
		return normalizeConsoleString(fieldKey, v)
	case json.Number:
		if shouldRedactPasswordField(fieldKey) {
			return maskedPasswordValue
		}
		return v.String()
	case []byte:
		return normalizeConsoleString(fieldKey, string(v))
	default:
		toJSON, err := json.Marshal(v)
		if err == nil {
			if redacted, ok := compactAndRedactJSON(string(toJSON)); ok {
				return redacted
			}
			return string(toJSON)
		}
		if shouldRedactPasswordField(fieldKey) {
			return maskedPasswordValue
		}
		if shouldRedactEmailField(fieldKey) {
			return maskEmailDomain(fmt.Sprint(v))
		}
		return fmt.Sprint(v)
	}
}

func trimTrailingSpaces(buf *bytes.Buffer) {
	for buf.Len() > 0 {
		last := buf.Bytes()[buf.Len()-1]
		if last != ' ' && last != '\t' {
			return
		}
		buf.Truncate(buf.Len() - 1)
	}
}

func normalizeConsoleString(fieldKey, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	if shouldRedactPasswordField(fieldKey) {
		return maskedPasswordValue
	}

	if shouldRedactEmailField(fieldKey) {
		return maskEmailDomain(trimmed)
	}

	if compacted, ok := compactAndRedactJSON(trimmed); ok {
		return compacted
	}

	if strings.ContainsAny(trimmed, "\r\n\t") {
		return strings.Join(strings.Fields(trimmed), " ")
	}

	return trimmed
}

func compactAndRedactJSON(value string) (string, bool) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()

	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return "", false
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "", false
	}

	redacted, err := json.Marshal(redactStructuredValue("", payload))
	if err != nil {
		return "", false
	}

	return string(redacted), true
}

func redactStructuredValue(fieldKey string, value any) any {
	if shouldRedactPasswordField(fieldKey) {
		return maskedPasswordValue
	}

	switch v := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(v))
		for key, nestedValue := range v {
			redacted[key] = redactStructuredValue(key, nestedValue)
		}
		return redacted
	case []any:
		redacted := make([]any, len(v))
		for index, nestedValue := range v {
			redacted[index] = redactStructuredValue(fieldKey, nestedValue)
		}
		return redacted
	case string:
		if shouldRedactEmailField(fieldKey) {
			return maskEmailDomain(v)
		}
		if compacted, ok := compactAndRedactJSON(strings.TrimSpace(v)); ok {
			return compacted
		}
		return v
	default:
		return value
	}
}

func shouldRedactEmailField(fieldKey string) bool {
	normalized := normalizeSensitiveFieldKey(fieldKey)
	return normalized == "email" || normalized == "emailaddress" || strings.HasSuffix(normalized, "email")
}

func shouldRedactPasswordField(fieldKey string) bool {
	normalized := normalizeSensitiveFieldKey(fieldKey)
	return normalized == "password" || normalized == "passwd" || normalized == "pwd" || strings.HasSuffix(normalized, "password")
}

func normalizeSensitiveFieldKey(fieldKey string) string {
	var builder strings.Builder
	builder.Grow(len(fieldKey))

	for _, char := range fieldKey {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(unicode.ToLower(char))
		}
	}

	return builder.String()
}

func maskEmailDomain(value string) string {
	localPart, domainPart, ok := strings.Cut(strings.TrimSpace(value), "@")
	if !ok || localPart == "" || domainPart == "" {
		return maskedPasswordValue
	}

	return localPart + "@*****"
}

// newLogger creates a zerolog.Logger configured for env.
//
// Dev:  Pretty console output with colors · Debug level · caller info
// Prod: JSON to stdout · Info level · caller info
//
// Both environments include timestamp and caller information.
func newLogger(env Environment, appName, version string) zerolog.Logger {
	return newLoggerWithWriter(newWriter(env, appName, version), env)
}

// newLoggerWithWriter builds a zerolog.Logger writing to w, with level and
// format determined by env. Used by Init when composing a MultiLevelWriter.
func newLoggerWithWriter(w io.Writer, env Environment) zerolog.Logger {
	// Set global level
	switch env {
	case Prod:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	return zerolog.New(newRedactingWriter(w)).With().Timestamp().Caller().Logger()
}

type redactingWriter struct {
	next io.Writer
}

func newRedactingWriter(next io.Writer) io.Writer {
	return redactingWriter{next: next}
}

func (w redactingWriter) Write(p []byte) (int, error) {
	trimmed := strings.TrimSpace(string(p))
	if trimmed == "" {
		return w.next.Write(p)
	}

	redacted, ok := compactAndRedactJSON(trimmed)
	if !ok {
		return w.next.Write(p)
	}

	if len(p) > 0 && p[len(p)-1] == '\n' {
		redacted += "\n"
	}

	_, err := io.WriteString(w.next, redacted)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}
