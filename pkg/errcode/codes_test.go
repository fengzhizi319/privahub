package errcode

import (
	"errors"
	"testing"
)

func TestJavaAlignedCodes(t *testing.T) {
	tests := []struct {
		name string
		code *ErrorCode
		want int
	}{
		{"validation", SystemValidationError, 202011100},
		{"unknown", SystemUnknownError, 202011101},
		{"auth failed", AuthFailed, 202011602},
		{"user password error", AuthUserPasswordError, 202011601},
		{"user locked", AuthUserIsLocked, 202011603},
		{"reset pwd locked", AuthResetPasswordIsLocked, 202011604},
		{"pwd same", UserUpdatePasswordErrorSame, 202012002},
		{"project not exists", ProjectNotExists, 202011501},
		{"remote call", SystemRemoteCallError, 2020111011},
		// legacy aliases now carry Java numbers
		{"legacy system", SystemError, 202011101},
		{"legacy param", ParamError, 202011100},
		{"legacy unauthorized", Unauthorized, 202011602},
		{"legacy forbidden", Forbidden, 202011602},
		{"legacy project", ProjectNotFound, 202011501},
		{"legacy node", NodeNotFound, 202011403},
	}
	for _, tt := range tests {
		if tt.code.Code != tt.want {
			t.Errorf("%s: code = %d, want %d", tt.name, tt.code.Code, tt.want)
		}
	}
}

func TestAllJavaKeysHaveMessages(t *testing.T) {
	for key, m := range messages {
		if m[0] == "" || m[1] == "" {
			t.Errorf("key %s missing a translation", key)
		}
	}
	if len(messages) < 140 {
		t.Errorf("expected ~142 Java messages, got %d", len(messages))
	}
}

func TestLocalize(t *testing.T) {
	if got := AuthUserPasswordError.Localize(LangEN, "3"); got == "" || got == AuthUserPasswordError.Key {
		t.Errorf("unexpected en message %q", got)
	}
	zh := SystemValidationError.Localize(LangZH, "name is empty")
	if zh != "入参校验失败: name is empty" {
		t.Errorf("zh = %q", zh)
	}
	// Missing arg drops the placeholder and trailing separator.
	if got := SystemUnknownError.Localize(LangZH); got != "系统未知错误" {
		t.Errorf("no-arg zh = %q", got)
	}
	if got := SystemUnknownError.Localize(LangEN); got != "System unknown error" {
		t.Errorf("no-arg en = %q", got)
	}
	// Go-only codes return their fixed message (or the explicit arg).
	custom := New(1, "custom")
	if custom.Localize(LangZH) != "custom" || custom.Localize(LangZH, "x") != "x" {
		t.Error("custom code localization wrong")
	}
}

func TestParseAcceptLanguage(t *testing.T) {
	cases := map[string]string{
		"":                        DefaultLang,
		"en-US,en;q=0.9":          LangEN,
		"zh-CN,zh;q=0.9,en;q=0.8": LangZH,
		"fr-FR, en;q=0.5":         LangEN,
		"de":                      DefaultLang,
	}
	for in, want := range cases {
		if got := ParseAcceptLanguage(in); got != want {
			t.Errorf("ParseAcceptLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNew(t *testing.T) {
	err := New(12345, "custom error")
	if err.Code != 12345 || err.Message != "custom error" {
		t.Errorf("unexpected %+v", err)
	}
	if err.Error() != "[12345] custom error" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestErrorCode_ImplementsError(t *testing.T) {
	var err error = SystemError
	var ec *ErrorCode
	if !errors.As(err, &ec) || ec.Code != SystemError.Code {
		t.Error("errors.As should work with ErrorCode")
	}
}

func TestJavaCodes_Unique(t *testing.T) {
	seen := map[int]string{}
	for key := range messages {
		_ = key
	}
	for _, ec := range []*ErrorCode{
		SystemValidationError, SystemUnknownError, AuthUserNotFound, AuthUserPasswordError,
		AuthFailed, AuthUserIsLocked, AuthResetPasswordIsLocked, ProjectNotExists,
		UserUpdatePasswordErrorSame, UserUpdatePasswordErrorInconsistent,
	} {
		if k, ok := seen[ec.Code]; ok {
			t.Errorf("duplicate code %d (%s, %s)", ec.Code, k, ec.Key)
		}
		seen[ec.Code] = ec.Key
	}
}
