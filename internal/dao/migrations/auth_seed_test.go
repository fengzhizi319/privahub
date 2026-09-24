package migration

import (
	"context"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"go.uber.org/zap"
)

func TestSeedAdmin_ProductionRandomPasswordFile(t *testing.T) {
	db := setupMigrateTestDB(t)
	file := filepath.Join(t.TempDir(), "sub", "initial-admin-password")
	opts := SeedOptions{Dev: false, InitialPasswordFile: file, PlatformType: "EDGE", NodeID: "alice"}
	if err := SeedDataWithOptions(context.Background(), db, zap.NewNop(), opts); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(file)
	if err != nil {
		t.Fatalf("password file not written: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("password file mode = %v", st.Mode().Perm())
	}
	b, _ := os.ReadFile(file)
	pwd := strings.TrimSpace(string(b))
	if len(pwd) != 16 {
		t.Errorf("unexpected password length %d", len(pwd))
	}
	var admin model.UserAccountsDO
	db.Where("name = ?", "admin").First(&admin)
	if !kdfMatches(admin.PasswordHash, pwd) || kdfMatches(admin.PasswordHash, "12345678") {
		t.Error("admin hash must be the random password")
	}
	if !admin.MustChangePassword {
		t.Error("a generated initial admin password must be changed at first login")
	}
	if admin.OwnerType != "EDGE" || admin.OwnerID != "alice" {
		t.Errorf("owner = %s/%s", admin.OwnerType, admin.OwnerID)
	}
	// Second run leaves the account untouched and does not rewrite the file.
	_ = os.Remove(file)
	if err := SeedDataWithOptions(context.Background(), db, zap.NewNop(), opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err == nil {
		t.Error("existing admin must not get a new password")
	}
}

func TestSeedAdmin_ConfiguredPassword(t *testing.T) {
	db := setupMigrateTestDB(t)
	opts := SeedOptions{AdminName: "root", AdminPassword: "S3cret!", PlatformType: "AUTONOMY", NodeID: "n1",
		InitialPasswordFile: filepath.Join(t.TempDir(), "p")}
	if err := SeedDataWithOptions(context.Background(), db, zap.NewNop(), opts); err != nil {
		t.Fatal(err)
	}
	var u model.UserAccountsDO
	if err := db.Where("name = ?", "root").First(&u).Error; err != nil {
		t.Fatal(err)
	}
	if !kdfMatches(u.PasswordHash, "S3cret!") || u.OwnerType != "P2P" || u.MustChangePassword {
		t.Errorf("unexpected admin %+v", u)
	}
}

func TestSeedRBAC_JavaRolesAndNodePermissions(t *testing.T) {
	db := setupMigrateTestDB(t)
	if err := SeedData(context.Background(), db, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&model.SysResourceDO{}).Count(&n)
	if n < 79 {
		t.Errorf("expected >=79 API resources, got %d", n)
	}
	for role, min := range map[string]int64{"EDGE_USER": 46, "EDGE_NODE": 16, "ADMIN": 1} {
		db.Model(&model.SysRoleResourceRelDO{}).Where("role_code = ?", role).Count(&n)
		if n < min {
			t.Errorf("role %s has %d resources, want >= %d", role, n, min)
		}
	}
	db.Model(&model.SysUserPermissionRelDO{}).Where("user_key = ? AND user_type = ? AND target_code = ?", "alice", "NODE", "EDGE_NODE").Count(&n)
	if n != 1 {
		t.Errorf("seeded node alice should get EDGE_NODE, got %d", n)
	}
	// Idempotent.
	if err := SeedData(context.Background(), db, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.SysRoleResourceRelDO{}).Where("role_code = ?", "EDGE_USER").Count(&n)
	if n != 46 {
		t.Errorf("reseeding duplicated rels: %d", n)
	}
}

func kdfMatches(stored, plain string) bool {
	return auth.VerifyStoredPassword(stored, auth.PasswordCandidates{SHA256: hashPassword(plain)}).OK
}
