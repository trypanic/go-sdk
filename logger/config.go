package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

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
		if shouldRedactSecretField(fieldKey) {
			return maskedPasswordValue
		}
		return v.String()
	case []byte:
		return normalizeConsoleString(fieldKey, string(v))
	default:
		toJSON, err := json.Marshal(v)
		if err == nil {
			if redacted, ok := compactAndRedactJSON(string(toJSON), nil); ok {
				return redacted
			}
			return string(toJSON)
		}
		if shouldRedactSecretField(fieldKey) {
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

	if shouldRedactSecretField(fieldKey) {
		return maskedPasswordValue
	}

	if shouldRedactEmailField(fieldKey) {
		return maskEmailDomain(trimmed)
	}

	if compacted, ok := compactAndRedactJSON(trimmed, nil); ok {
		return compacted
	}

	if strings.ContainsAny(trimmed, "\r\n\t") {
		return strings.Join(strings.Fields(trimmed), " ")
	}

	return trimmed
}

// compactAndRedactJSON decodes value as a single JSON document, applies the
// redaction policy (see redactStructuredValue) and re-encodes it compactly.
// It reports false when value is not exactly one JSON document.
func compactAndRedactJSON(value string, custom func(key string, value any) any) (string, bool) {
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

	redacted, err := json.Marshal(redactStructuredValue("", payload, custom))
	if err != nil {
		return "", false
	}

	return string(redacted), true
}

// redactStructuredValue applies the built-in redaction policy to value,
// the value of the field fieldKey, recursively:
//
//   - a sensitive key (see shouldRedactSecretField) masks the whole value;
//   - an email key keeps only the local part of the address;
//   - a string holding a JSON document is redacted recursively;
//   - any other string has the password of every URL userinfo masked.
//
// When custom is non-nil it is called for every object field after the
// built-in policy, with the already-redacted value, and its result is
// stored; it can add masking but cannot undo the built-in one.
func redactStructuredValue(fieldKey string, value any, custom func(key string, value any) any) any {
	if shouldRedactSecretField(fieldKey) {
		return maskedPasswordValue
	}

	switch v := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(v))
		for key, nestedValue := range v {
			nested := redactStructuredValue(key, nestedValue, custom)
			if custom != nil {
				nested = custom(key, nested)
			}
			redacted[key] = nested
		}
		return redacted
	case []any:
		redacted := make([]any, len(v))
		for index, nestedValue := range v {
			redacted[index] = redactStructuredValue(fieldKey, nestedValue, custom)
		}
		return redacted
	case string:
		if shouldRedactEmailField(fieldKey) {
			return maskEmailDomain(v)
		}
		if trimmed := strings.TrimSpace(v); looksLikeJSONContainer(trimmed) {
			if compacted, ok := compactAndRedactJSON(trimmed, custom); ok {
				return compacted
			}
		}
		return maskURLUserinfo(v)
	default:
		return value
	}
}

// looksLikeJSONContainer reports whether s may hold a JSON object or array,
// the only string values worth decoding for nested redaction.
func looksLikeJSONContainer(s string) bool {
	return s != "" && (s[0] == '{' || s[0] == '[')
}

// urlUserinfoPassword matches "scheme://user:password@" so the password can
// be masked wherever a URL appears inside a string value.
var urlUserinfoPassword = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.\-]*://[^\s/?#@:]*):[^\s/?#@]*@`)

// maskURLUserinfo replaces the password of every URL userinfo in s with
// the mask, keeping the scheme, user and host.
func maskURLUserinfo(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	return urlUserinfoPassword.ReplaceAllString(s, "${1}:"+maskedPasswordValue+"@")
}

func shouldRedactEmailField(fieldKey string) bool {
	normalized := normalizeSensitiveFieldKey(fieldKey)
	return normalized == "email" || normalized == "emailaddress" || strings.HasSuffix(normalized, "email")
}

// sensitiveKeySuffixes lists the normalized (lower-case, letters and digits
// only) key suffixes whose values are always masked: "Proxy-Authorization",
// "set_cookie", "X-Api-Key" or "postgres_dsn" all match.
var sensitiveKeySuffixes = []string{
	"password", "passwd", "pwd",
	"secret",
	"token",
	"authorization",
	"cookie",
	"apikey",
	"privatekey",
	"credential", "credentials",
	"dsn",
}

// shouldRedactSecretField reports whether the value of fieldKey must be
// masked entirely.
func shouldRedactSecretField(fieldKey string) bool {
	normalized := normalizeSensitiveFieldKey(fieldKey)
	for _, suffix := range sensitiveKeySuffixes {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
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
// Dev:  Pretty console output with colors · Debug level
// Prod: JSON to stdout · Info level
//
// Both environments include timestamp information.
func newLogger(env Environment, appName, version string) zerolog.Logger {
	return newLoggerWithWriter(newWriter(env, appName, version), env)
}

// newLoggerWithWriter builds a zerolog.Logger writing to w, with level and
// format determined by env. Used by Init and New.
//
// It never touches zerolog's process-wide state: the level is set on the
// instance and the timestamp is written by a per-logger hook, so loggers
// built concurrently or with different environments do not affect each
// other (or any other zerolog logger in the process).
func newLoggerWithWriter(w io.Writer, env Environment) zerolog.Logger {
	return newLoggerWithRedactor(w, env, nil)
}

// newLoggerWithRedactor is newLoggerWithWriter with an optional custom
// redactor applied after the built-in policy (Config.Redactor).
func newLoggerWithRedactor(w io.Writer, env Environment, custom func(key string, value any) any) zerolog.Logger {
	return zerolog.New(newRedactingWriter(w, custom)).
		Level(levelFor(env)).
		Hook(timestampHook{})
}

// levelFor returns the minimum level a logger built for env emits.
func levelFor(env Environment) zerolog.Level {
	if env == Prod {
		return zerolog.InfoLevel
	}
	return zerolog.DebugLevel
}

// timestampHook stamps every event with the current time as Unix epoch
// seconds, independently of zerolog.TimeFieldFormat.
type timestampHook struct{}

func (timestampHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	e.Int64(zerolog.TimestampFieldName, zerolog.TimestampFunc().Unix())
}

// applyGlobalSettings sets zerolog's process-wide level and time format for
// env. Only Init calls it: Init is the one SDK function allowed to mutate
// zerolog globals (AGENTS.md §2.3).
func applyGlobalSettings(env Environment) {
	zerolog.SetGlobalLevel(levelFor(env))
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
}

// redactingWriter applies the redaction policy to every JSON log line before
// passing it to next.
type redactingWriter struct {
	next   io.Writer
	custom func(key string, value any) any
}

func newRedactingWriter(next io.Writer, custom func(key string, value any) any) io.Writer {
	return redactingWriter{next: next, custom: custom}
}

func (w redactingWriter) Write(p []byte) (int, error) {
	if w.custom == nil && !mayNeedRedaction(p) {
		return w.next.Write(p)
	}

	trimmed := strings.TrimSpace(string(p))
	if trimmed == "" {
		return w.next.Write(p)
	}

	redacted, ok := compactAndRedactJSON(trimmed, w.custom)
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

// redactionNeedles are the normalized substrings whose presence anywhere in
// a log line sends it through the full (decode, redact, re-encode) path:
// every sensitive key suffix plus "email".
var redactionNeedles = func() [][]byte {
	needles := make([][]byte, 0, len(sensitiveKeySuffixes)+1)
	for _, suffix := range sensitiveKeySuffixes {
		needles = append(needles, []byte(suffix))
	}
	return append(needles, []byte("email"))
}()

var (
	urlSchemeSeparator = []byte("://")
	jsonUnicodeEscape  = []byte(`\u`)
)

// normalizedLinePool recycles the scratch buffers used by mayNeedRedaction.
var normalizedLinePool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

// mayNeedRedaction is a cheap, conservative pre-filter: it returns false only
// when the built-in policy cannot change line, i.e. the line contains no URL,
// no JSON \u escape (which could hide a key) and, after the same
// normalization applied to keys (lower case, letters and digits only), none
// of redactionNeedles. Lines it rejects are written through untouched.
func mayNeedRedaction(line []byte) bool {
	if bytes.Contains(line, urlSchemeSeparator) || bytes.Contains(line, jsonUnicodeEscape) {
		return true
	}

	bufPtr := normalizedLinePool.Get().(*[]byte)
	normalized := appendNormalized((*bufPtr)[:0], line)
	found := false
	for _, needle := range redactionNeedles {
		if bytes.Contains(normalized, needle) {
			found = true
			break
		}
	}
	*bufPtr = normalized
	normalizedLinePool.Put(bufPtr)
	return found
}

// appendNormalized appends the ASCII letters and digits of line, lower-cased,
// to dst. Non-ASCII runes are lower-cased with unicode.ToLower (as
// normalizeSensitiveFieldKey does) and kept only when the result is ASCII,
// so every needle present in a normalized key is present in the output.
func appendNormalized(dst, line []byte) []byte {
	for i := 0; i < len(line); {
		c := line[i]
		if c < utf8.RuneSelf {
			i++
			switch {
			case 'a' <= c && c <= 'z', '0' <= c && c <= '9':
				dst = append(dst, c)
			case 'A' <= c && c <= 'Z':
				dst = append(dst, c+('a'-'A'))
			}
			continue
		}
		r, size := utf8.DecodeRune(line[i:])
		i += size
		if lower := unicode.ToLower(r); lower < utf8.RuneSelf && (unicode.IsLetter(lower) || unicode.IsDigit(lower)) {
			dst = append(dst, byte(lower))
		}
	}
	return dst
}
