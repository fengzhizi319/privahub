package service

import (
	"errors"
	"fmt"

	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// BizError is a service-layer error carrying a Java-aligned error code
// (Java SecretpadException.of(code, cause, args...)). Handlers map it to the
// response envelope via its Code and Msg.
type BizError struct {
	Code *errcode.ErrorCode
	Msg  string
	Err  error
}

func (e *BizError) Error() string {
	msg := e.Msg
	if msg == "" && e.Code != nil {
		msg = e.Code.Message
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", msg, e.Err)
	}
	return msg
}

func (e *BizError) Unwrap() error { return e.Err }

// Is lets errors.Is(err, errcode.X) match on the numeric code.
func (e *BizError) Is(target error) bool {
	var ec *errcode.ErrorCode
	if errors.As(target, &ec) && e.Code != nil {
		return ec.Code == e.Code.Code
	}
	return false
}

func bizErr(code *errcode.ErrorCode, msg string) error {
	return &BizError{Code: code, Msg: msg}
}

func bizWrap(code *errcode.ErrorCode, err error, msg string) error {
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return &BizError{Code: code, Msg: msg, Err: err}
}

// AsBizError extracts a *BizError from err.
func AsBizError(err error) (*BizError, bool) {
	var be *BizError
	if errors.As(err, &be) {
		return be, true
	}
	return nil, false
}
