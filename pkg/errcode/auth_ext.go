// Copyright 2026 The Privahub Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package errcode

// Go-only authentication codes (security hardening). They extend the Java
// auth.* range 2020116xx without reusing a Java number, except
// AuthLoginFailed which deliberately keeps Java's USER_PASSWORD_ERROR number
// (202011601) so existing clients keep matching it, but with a message that
// no longer discloses the remaining attempts.
var (
	AuthLoginFailed = defineExt(202011601, "auth.LOGIN_FAILED",
		"用户名或密码错误", "Username or password error")
	AuthPasswordChangeRequired = defineExt(202011605, "auth.PASSWORD_CHANGE_REQUIRED",
		"请先修改初始密码或已被重置的密码", "Password change required before continuing")
	AuthPasswordTooWeak = defineExt(202011606, "auth.PASSWORD_TOO_WEAK",
		"密码过于简单（命中弱口令字典），请更换", "The password is too weak (found in the weak password dictionary)")
	AuthPasswordReused = defineExt(202011607, "auth.PASSWORD_REUSED",
		"新密码不能与最近使用过的密码相同", "The new password must differ from recently used passwords")
	AuthCSRFFailed = defineExt(202011608, "auth.CSRF_FAILED",
		"请求来源校验失败: {0}", "Request origin verification failed: {0}")
)

// defineExt registers a Go-only i18n message and defines the code.
func defineExt(code int, key, zh, en string) *ErrorCode {
	messages[key] = [2]string{zh, en}
	return define(code, key)
}
