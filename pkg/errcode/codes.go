// Package errcode defines unified error codes compatible with the Java SecretPad backend.
//
// The numeric codes and message keys mirror secretpad-common's *ErrorCode enums
// (see java_codes.go, generated from the Java sources) and messages are localized
// from the same zh_CN / en_US property files. Responses keep the Java envelope
// {"status":{"code":<code>,"msg":<localized message>}} with HTTP 200, except
// authentication failures on protected routes which additionally use HTTP 401 so
// the frontend can detect an expired session (see middleware/auth.go).
package errcode

import (
	"fmt"
	"strconv"
	"strings"
)

// ErrorCode represents a structured application error with code and message.
type ErrorCode struct {
	Code int `json:"code"`
	// Message is the default (English, placeholder-free) message, used by Error().
	Message string `json:"message"`
	// Key is the Java i18n message key (e.g. "auth.AUTH_FAILED"); empty for Go-only codes.
	Key string `json:"-"`
}

func (e *ErrorCode) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// New creates a new ErrorCode with a fixed message (Go-only code, not localized).
func New(code int, message string) *ErrorCode {
	return &ErrorCode{Code: code, Message: message}
}

// define creates a Java-aligned error code whose message is resolved from the
// i18n table by key.
func define(code int, key string) *ErrorCode {
	ec := &ErrorCode{Code: code, Key: key}
	if m, ok := messages[key]; ok {
		ec.Message = Format(m[1])
	} else {
		ec.Message = key
	}
	return ec
}

// alias returns a copy of a Java code with a Go-specific default message. The
// numeric code and i18n key stay identical to the Java one.
func alias(base *ErrorCode, message string) *ErrorCode {
	return &ErrorCode{Code: base.Code, Message: message, Key: base.Key}
}

// Legacy Go names kept for existing call sites. They now map onto the Java
// numeric codes (previously they used Go-private numbers 202011500..202011520,
// which collided with Java's project.* codes).
var (
	Success           = New(0, "success")
	SystemError       = SystemUnknownError
	ParamError        = SystemValidationError
	Unauthorized      = AuthFailed
	Forbidden         = alias(AuthFailed, "permission denied")
	NotFound          = alias(SystemHttp404Error, "resource not found")
	AlreadyExists     = alias(SystemValidationError, "resource already exists")
	TokenExpired      = alias(AuthFailed, "login is expire, please login again.")
	TokenInvalid      = alias(AuthFailed, "login is required")
	UserLocked        = AuthUserIsLocked
	PasswordError     = AuthUserPasswordError
	ProjectNotFound   = ProjectNotExists
	JobNotFound       = ProjectJobNotExists
	NodeNotFound      = NodeNotExistError
	RouteNotFound     = NodeRouteNotExistError
	VoteNotFound      = VoteNotExists
	DatatableNotFound = DatatableNotExists
	GraphNotFound     = GraphNotExists
	DAGHasCycle       = alias(GraphJobInvalid, "DAG contains cycle")
	KusciaConnError   = KusciaRpcError
	VoteExpired       = alias(VoteCheckFailed, "vote has expired")
	VoteRejected      = alias(VoteCheckFailed, "vote rejected")
)

// Supported languages.
const (
	LangZH = "zh"
	LangEN = "en"
)

// DefaultLang is used when the request carries no (supported) Accept-Language,
// matching Java's LocaleMessageResolver fallback to SIMPLIFIED_CHINESE.
var DefaultLang = LangZH

// ParseAcceptLanguage returns LangZH or LangEN for an Accept-Language header
// value, honoring the first supported language in preference order.
func ParseAcceptLanguage(header string) string {
	for _, part := range strings.Split(header, ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		switch {
		case strings.HasPrefix(tag, "zh"):
			return LangZH
		case strings.HasPrefix(tag, "en"):
			return LangEN
		}
	}
	return DefaultLang
}

// Localize renders the message for lang (LangZH/LangEN) with MessageFormat
// style positional args ({0}, {1}, ...). Placeholders without an argument are
// dropped together with a dangling ":" separator. Go-only codes (no Key) return
// their fixed message.
func (e *ErrorCode) Localize(lang string, args ...string) string {
	if e == nil {
		return ""
	}
	tmpl, ok := messages[e.Key]
	if !ok {
		if len(args) > 0 && args[0] != "" {
			return args[0]
		}
		return e.Message
	}
	t := tmpl[0]
	if lang == LangEN {
		t = tmpl[1]
	}
	return Format(t, args...)
}

// HasPlaceholder reports whether the code's message template takes arguments.
func (e *ErrorCode) HasPlaceholder() bool {
	if e == nil {
		return false
	}
	tmpl, ok := messages[e.Key]
	return ok && strings.Contains(tmpl[1], "{0}")
}

// Format substitutes {n} placeholders in tmpl. Missing arguments are removed
// together with a preceding ":"/"：" separator and surrounding whitespace.
func Format(tmpl string, args ...string) string {
	var b strings.Builder
	for i := 0; i < len(tmpl); i++ {
		if tmpl[i] == '{' {
			end := strings.IndexByte(tmpl[i:], '}')
			if end > 1 {
				if n, err := strconv.Atoi(tmpl[i+1 : i+end]); err == nil {
					if n < len(args) {
						b.WriteString(args[n])
					}
					i += end
					continue
				}
			}
		}
		b.WriteByte(tmpl[i])
	}
	out := b.String()
	if len(args) == 0 || strings.Contains(tmpl, "{"+strconv.Itoa(len(args))+"}") {
		out = strings.TrimRight(out, " ")
		out = strings.TrimSuffix(out, ":")
		out = strings.TrimSuffix(out, "：")
		out = strings.TrimRight(out, " ")
	}
	return out
}
