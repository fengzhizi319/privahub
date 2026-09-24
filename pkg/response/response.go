// Package response provides unified HTTP response formatting compatible with Java SecretPad.
package response

import (
	"net/http"

	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
)

// Body is the standard API response envelope matching Java SecretPad format.
type Body struct {
	Status *Status     `json:"status"`
	Data   interface{} `json:"data,omitempty"`
}

// Status contains error code and message.
type Status struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// Lang returns the negotiated message language (errcode.LangZH/LangEN) for the
// request based on its Accept-Language header (Java LocaleMessageResolver parity).
func Lang(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return errcode.DefaultLang
	}
	return errcode.ParseAcceptLanguage(c.GetHeader("Accept-Language"))
}

// Message renders the localized message of ec for the current request.
func Message(c *gin.Context, ec *errcode.ErrorCode, args ...string) string {
	return ec.Localize(Lang(c), args...)
}

// OK sends a successful response with data.
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Body{
		Status: &Status{Code: 0, Msg: "success"},
		Data:   data,
	})
}

// OKEmpty sends a successful response without data.
func OKEmpty(c *gin.Context) {
	c.JSON(http.StatusOK, Body{
		Status: &Status{Code: 0, Msg: "success"},
	})
}

// Fail sends an error response using a predefined ErrorCode. The message is
// localized from the Java i18n table according to Accept-Language.
func Fail(c *gin.Context, ec *errcode.ErrorCode) {
	c.JSON(http.StatusOK, Body{
		Status: &Status{Code: ec.Code, Msg: Message(c, ec)},
	})
}

// FailArgs sends an error response whose localized message template is filled
// with args ({0}, {1}, ...), like Java SecretpadException.of(code, args...).
func FailArgs(c *gin.Context, ec *errcode.ErrorCode, args ...string) {
	c.JSON(http.StatusOK, Body{
		Status: &Status{Code: ec.Code, Msg: Message(c, ec, args...)},
	})
}

// FailWithMsg sends an error response with a custom message. When the code's
// Java template takes an argument (e.g. "入参校验失败: {0}") the message is used
// as {0}; otherwise the custom message is returned verbatim.
func FailWithMsg(c *gin.Context, ec *errcode.ErrorCode, msg string) {
	out := msg
	if ec.HasPlaceholder() {
		out = Message(c, ec, msg)
	}
	c.JSON(http.StatusOK, Body{
		Status: &Status{Code: ec.Code, Msg: out},
	})
}

// FailHTTP sends an error with specific HTTP status code.
func FailHTTP(c *gin.Context, httpCode int, ec *errcode.ErrorCode, args ...string) {
	c.JSON(httpCode, Body{
		Status: &Status{Code: ec.Code, Msg: Message(c, ec, args...)},
	})
}

// AbortHTTP is FailHTTP + c.Abort (for middleware).
func AbortHTTP(c *gin.Context, httpCode int, ec *errcode.ErrorCode, args ...string) {
	c.AbortWithStatusJSON(httpCode, Body{
		Status: &Status{Code: ec.Code, Msg: Message(c, ec, args...)},
	})
}
