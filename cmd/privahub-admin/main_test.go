package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/fengzhizi319/privahub/internal/audit"
	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	pauth "github.com/fengzhizi319/privahub/pkg/auth"
)

func setupDB(t *testing.T) (string, *gorm.DB) {
	t.Helper()
	pauth.SetPasswordKDFIterationsForTesting(1000)
	t.Cleanup(func() { pauth.SetPasswordKDFIterationsForTesting(0) })
	path := filepath.Join(t.TempDir(), "p.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := migration.AutoMigrate(db, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	if err := migration.SeedDataWithOptions(context.Background(), db, zap.NewNop(), migration.DevSeedOptions()); err != nil {
		t.Fatal(err)
	}
	return path, db
}

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(context.Background(), args, &out, &errb)
	return code, out.String(), errb.String()
}

var otpRe = regexp.MustCompile(`one-time password for admin: (\S+)`)

func TestResetPassword_Stdout(t *testing.T) {
	path, db := setupDB(t)
	now := time.Now()
	// an active session and a lockout that must be cleared
	if err := db.Create(&model.UserTokensDO{Name: "admin", Token: "digest", SessionID: "s1", GmtToken: &now}).Error; err != nil {
		t.Fatal(err)
	}
	locked := now.Add(time.Hour)
	fa := 9
	db.Model(&model.UserAccountsDO{}).Where("name = ?", "admin").Updates(map[string]any{"failed_attempts": fa, "locked_invalid_time": locked})
	var before model.UserAccountsDO
	db.Where("name = ?", "admin").Take(&before)

	code, out, errOut := runCLI("reset-password", "--driver", "sqlite", "--dsn", path, "--user", "admin",
		"--reason", "INC-42 admin lost password", "--operator", "alice")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	m := otpRe.FindStringSubmatch(out)
	if m == nil || len(m[1]) != 20 {
		t.Fatalf("password not printed once: %q", out)
	}
	if strings.Count(out, m[1]) != 1 {
		t.Fatal("password printed more than once")
	}
	var u model.UserAccountsDO
	db.Where("name = ?", "admin").Take(&u)
	if !u.MustChangePassword || u.PasswordVersion != before.PasswordVersion+1 {
		t.Errorf("must_change=%v pv=%d (before %d)", u.MustChangePassword, u.PasswordVersion, before.PasswordVersion)
	}
	if u.FailedAttempts != nil || u.LockedInvalidTime != nil {
		t.Error("lockout not cleared")
	}
	if !strings.HasPrefix(u.PasswordHash, "sm3kdf:") {
		t.Errorf("password not stored with the KDF: %q", u.PasswordHash)
	}
	if !pauth.VerifyStoredPassword(u.PasswordHash, pauth.PasswordCandidates{SHA256: service.HashPassword(m[1])}).OK {
		t.Error("stored hash does not verify the printed password")
	}
	var sessions int64
	db.Model(&model.UserTokensDO{}).Where("name = ?", "admin").Count(&sessions)
	if sessions != 0 {
		t.Errorf("sessions not revoked: %d", sessions)
	}
	var ev model.AuditLogDO
	if err := db.Where("action = ? AND resource_id = ?", audit.ActionPasswordReset, "admin").Take(&ev).Error; err != nil {
		t.Fatalf("audit event missing: %v", err)
	}
	if ev.ActorType != audit.ActorSystem || ev.Reason != "INC-42 admin lost password" ||
		!strings.Contains(ev.Extra, "alice") || !strings.Contains(ev.Extra, service.BreakGlassMethod) {
		t.Errorf("audit event %+v", ev)
	}
	if strings.Contains(ev.Extra+ev.Reason, m[1]) {
		t.Error("password leaked into the audit trail")
	}
	res, err := audit.NewStore(db).Verify(context.Background(), audit.VerifyOptions{})
	if err != nil || !res.Valid {
		t.Fatalf("audit chain invalid after reset: %+v %v", res, err)
	}
}

func TestResetPassword_OutputFile(t *testing.T) {
	path, db := setupDB(t)
	f := filepath.Join(t.TempDir(), "otp.txt")
	code, out, errOut := runCLI("reset-password", "-driver", "sqlite", "-dsn", path, "-user", "admin", "-reason", "INC-43", "-output-file", f)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	st, err := os.Stat(f)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("output file: %v %v", st, err)
	}
	b, _ := os.ReadFile(f)
	pw := strings.TrimSpace(string(b))
	if len(pw) != 20 || strings.Contains(out, pw) {
		t.Fatalf("password must only be in the file: out=%q", out)
	}
	var u model.UserAccountsDO
	db.Where("name = ?", "admin").Take(&u)
	if !pauth.VerifyStoredPassword(u.PasswordHash, pauth.PasswordCandidates{SHA256: service.HashPassword(pw)}).OK {
		t.Error("file password does not verify")
	}
	// never overwrites an existing file
	if code, _, _ := runCLI("reset-password", "-driver", "sqlite", "-dsn", path, "-user", "admin", "-reason", "x", "-output-file", f); code != 1 {
		t.Errorf("existing output file must fail, got %d", code)
	}
	var u2 model.UserAccountsDO
	db.Where("name = ?", "admin").Take(&u2)
	if u2.PasswordHash != u.PasswordHash {
		t.Error("failed run must not change the password")
	}
}

func TestResetPassword_Errors(t *testing.T) {
	path, db := setupDB(t)
	var before model.UserAccountsDO
	db.Where("name = ?", "admin").Take(&before)
	cases := map[string]struct {
		args []string
		code int
	}{
		"no command":     {nil, 2},
		"unknown cmd":    {[]string{"frobnicate"}, 2},
		"missing reason": {[]string{"reset-password", "-driver", "sqlite", "-dsn", path, "-user", "admin"}, 2},
		"missing user":   {[]string{"reset-password", "-driver", "sqlite", "-dsn", path, "-reason", "x"}, 2},
		"unknown user":   {[]string{"reset-password", "-driver", "sqlite", "-dsn", path, "-user", "nobody", "-reason", "x"}, 1},
		"short length":   {[]string{"reset-password", "-driver", "sqlite", "-dsn", path, "-user", "admin", "-reason", "x", "-length", "8"}, 1},
		"bad driver":     {[]string{"reset-password", "-driver", "oracle", "-dsn", "x", "-user", "admin", "-reason", "x"}, 1},
	}
	for name, c := range cases {
		if code, _, _ := runCLI(c.args...); code != c.code {
			t.Errorf("%s: exit %d want %d", name, code, c.code)
		}
	}
	var after model.UserAccountsDO
	db.Where("name = ?", "admin").Take(&after)
	if after.PasswordHash != before.PasswordHash || after.MustChangePassword != before.MustChangePassword {
		t.Error("failed runs must not change the account")
	}
	var n int64
	db.Model(&model.AuditLogDO{}).Where("action = ?", audit.ActionPasswordReset).Count(&n)
	if n != 0 {
		t.Errorf("failed runs must not write reset audit events: %d", n)
	}
}
