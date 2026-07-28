// Package response provides unified HTTP response formatting compatible with Java SecretPad.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/pkg/errcode"
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

// Fail sends an error response using predefined ErrorCode.
func Fail(c *gin.Context, ec *errcode.ErrorCode) {
	c.JSON(http.StatusOK, Body{
		Status: &Status{Code: ec.Code, Msg: ec.Message},
	})
}

// FailWithMsg sends an error response with custom message.
func FailWithMsg(c *gin.Context, ec *errcode.ErrorCode, msg string) {
	c.JSON(http.StatusOK, Body{
		Status: &Status{Code: ec.Code, Msg: msg},
	})
}

// FailHTTP sends an error with specific HTTP status code.
func FailHTTP(c *gin.Context, httpCode int, ec *errcode.ErrorCode) {
	c.JSON(httpCode, Body{
		Status: &Status{Code: ec.Code, Msg: ec.Message},
	})
}
