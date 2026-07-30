package errcode

import (
	"errors"
	"testing"
)

func TestErrorCode_Error(t *testing.T) {
	tests := []struct {
		name     string
		code     *ErrorCode
		expected string
	}{
		{"success", Success, "[0] success"},
		{"system error", SystemError, "[202011500] system unknown error"},
		{"param error", ParamError, "[202011501] parameter validation error"},
		{"unauthorized", Unauthorized, "[202011502] unauthorized access"},
		{"not found", NotFound, "[202011504] resource not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.code.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	err := New(12345, "custom error")
	if err.Code != 12345 {
		t.Errorf("expected code 12345, got %d", err.Code)
	}
	if err.Message != "custom error" {
		t.Errorf("expected message 'custom error', got %q", err.Message)
	}
}

func TestErrorCode_ImplementsError(t *testing.T) {
	var err error = SystemError
	if err == nil {
		t.Error("ErrorCode should implement error interface")
	}

	// Test errors.Is compatibility
	var ec *ErrorCode
	if !errors.As(err, &ec) {
		t.Error("errors.As should work with ErrorCode")
	}
	if ec.Code != SystemError.Code {
		t.Errorf("expected code %d, got %d", SystemError.Code, ec.Code)
	}
}

func TestPredefinedCodes_Unique(t *testing.T) {
	codes := []*ErrorCode{
		Success, SystemError, ParamError, Unauthorized, Forbidden,
		NotFound, AlreadyExists, TokenExpired, TokenInvalid, UserLocked,
		PasswordError, ProjectNotFound, JobNotFound, NodeNotFound,
		RouteNotFound, VoteNotFound, DatatableNotFound, GraphNotFound,
		DAGHasCycle, KusciaConnError, VoteExpired, VoteRejected,
	}

	seen := make(map[int]string)
	for _, ec := range codes {
		if msg, exists := seen[ec.Code]; exists {
			t.Errorf("duplicate code %d: %q and %q", ec.Code, msg, ec.Message)
		}
		seen[ec.Code] = ec.Message
	}
}

func TestPredefinedCodes_NonNegative(t *testing.T) {
	codes := []*ErrorCode{
		Success, SystemError, ParamError, Unauthorized, Forbidden,
		NotFound, AlreadyExists, TokenExpired, TokenInvalid, UserLocked,
		PasswordError, ProjectNotFound, JobNotFound, NodeNotFound,
		RouteNotFound, VoteNotFound, DatatableNotFound, GraphNotFound,
		DAGHasCycle, KusciaConnError, VoteExpired, VoteRejected,
	}

	for _, ec := range codes {
		if ec.Code < 0 {
			t.Errorf("code %d should be non-negative", ec.Code)
		}
	}
}
