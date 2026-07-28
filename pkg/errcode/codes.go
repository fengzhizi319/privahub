// Package errcode defines unified error codes compatible with the Java SecretPad backend.
package errcode

import "fmt"

// ErrorCode represents a structured application error with code and message.
type ErrorCode struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorCode) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// New creates a new ErrorCode.
func New(code int, message string) *ErrorCode {
	return &ErrorCode{Code: code, Message: message}
}

// Predefined error codes matching Java SecretPad backend conventions.
var (
	Success           = New(0, "success")
	SystemError       = New(202011500, "system unknown error")
	ParamError        = New(202011501, "parameter validation error")
	Unauthorized      = New(202011502, "unauthorized access")
	Forbidden         = New(202011503, "permission denied")
	NotFound          = New(202011504, "resource not found")
	AlreadyExists     = New(202011505, "resource already exists")
	TokenExpired      = New(202011506, "token expired")
	TokenInvalid      = New(202011507, "token invalid")
	UserLocked        = New(202011508, "user account locked")
	PasswordError     = New(202011509, "username or password error")
	ProjectNotFound   = New(202011510, "project not found")
	JobNotFound       = New(202011511, "job not found")
	NodeNotFound      = New(202011512, "node not found")
	RouteNotFound     = New(202011513, "node route not found")
	VoteNotFound      = New(202011514, "vote not found")
	DatatableNotFound = New(202011515, "datatable not found")
	GraphNotFound     = New(202011516, "graph not found")
	DAGHasCycle       = New(202011517, "DAG contains cycle")
	KusciaConnError   = New(202011518, "kuscia connection error")
	VoteExpired       = New(202011519, "vote has expired")
	VoteRejected      = New(202011520, "vote rejected")
)
