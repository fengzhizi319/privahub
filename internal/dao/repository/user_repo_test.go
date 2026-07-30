package repository

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.UserAccountsDO{},
		&model.UserTokensDO{},
		&model.SysUserPermissionRelDO{},
		&model.SysUserNodeRelDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- UserAccountsRepo tests ---

func TestUserAccountsRepo_FindByName(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewUserAccountsRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.UserAccountsDO{Name: "admin", PasswordHash: "hash123"})

	found, err := repo.FindByName(ctx, "admin")
	if err != nil {
		t.Fatalf("FindByName failed: %v", err)
	}
	if found.PasswordHash != "hash123" {
		t.Errorf("expected password_hash 'hash123', got %q", found.PasswordHash)
	}
}

func TestUserAccountsRepo_FindByName_NotFound(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewUserAccountsRepo(db)

	_, err := repo.FindByName(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

func TestUserAccountsRepo_UpdatePassword(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewUserAccountsRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.UserAccountsDO{Name: "admin", PasswordHash: "old-hash"})

	err := repo.UpdatePassword(ctx, "admin", "new-hash")
	if err != nil {
		t.Fatalf("UpdatePassword failed: %v", err)
	}

	found, _ := repo.FindByName(ctx, "admin")
	if found.PasswordHash != "new-hash" {
		t.Errorf("expected password_hash 'new-hash', got %q", found.PasswordHash)
	}
}

// --- UserTokensRepo tests ---

func TestUserTokensRepo_FindByToken(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewUserTokensRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.UserTokensDO{Name: "admin", Token: "tok-abc"})

	found, err := repo.FindByToken(ctx, "tok-abc")
	if err != nil {
		t.Fatalf("FindByToken failed: %v", err)
	}
	if found.Name != "admin" {
		t.Errorf("expected name 'admin', got %q", found.Name)
	}
}

func TestUserTokensRepo_FindByToken_NotFound(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewUserTokensRepo(db)

	_, err := repo.FindByToken(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent token")
	}
}

func TestUserTokensRepo_FindByName(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewUserTokensRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.UserTokensDO{Name: "admin", Token: "tok-1"})
	repo.Create(ctx, &model.UserTokensDO{Name: "admin", Token: "tok-2"})
	repo.Create(ctx, &model.UserTokensDO{Name: "user", Token: "tok-3"})

	tokens, err := repo.FindByName(ctx, "admin")
	if err != nil {
		t.Fatalf("FindByName failed: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens for admin, got %d", len(tokens))
	}
}

func TestUserTokensRepo_DeleteByName(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewUserTokensRepo(db)
	ctx := context.Background()

	repo.Create(ctx, &model.UserTokensDO{Name: "admin", Token: "tok-1"})
	repo.Create(ctx, &model.UserTokensDO{Name: "admin", Token: "tok-2"})
	repo.Create(ctx, &model.UserTokensDO{Name: "user", Token: "tok-3"})

	err := repo.DeleteByName(ctx, "admin")
	if err != nil {
		t.Fatalf("DeleteByName failed: %v", err)
	}

	tokens, _ := repo.FindByName(ctx, "admin")
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens after delete, got %d", len(tokens))
	}

	// user tokens should be unaffected
	tokens, _ = repo.FindByName(ctx, "user")
	if len(tokens) != 1 {
		t.Errorf("expected 1 token for user, got %d", len(tokens))
	}
}

// --- SysUserPermissionRepo tests ---

func TestSysUserPermissionRepo_CRUD(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewSysUserPermissionRepo(db)
	ctx := context.Background()

	rel := &model.SysUserPermissionRelDO{UserType: "USER", UserKey: "admin", TargetType: "ROLE", TargetCode: "ADMIN"}
	err := repo.Create(ctx, rel)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	rels, err := repo.FindByUserKey(ctx, "admin")
	if err != nil {
		t.Fatalf("FindByUserKey failed: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 permission, got %d", len(rels))
	}
	if rels[0].TargetCode != "ADMIN" {
		t.Errorf("expected target_code 'ADMIN', got %q", rels[0].TargetCode)
	}

	err = repo.DeleteByUserKey(ctx, "admin")
	if err != nil {
		t.Fatalf("DeleteByUserKey failed: %v", err)
	}

	rels, _ = repo.FindByUserKey(ctx, "admin")
	if len(rels) != 0 {
		t.Errorf("expected 0 permissions after delete, got %d", len(rels))
	}
}

func TestSysUserPermissionRepo_FindByUserKey_Empty(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewSysUserPermissionRepo(db)

	rels, err := repo.FindByUserKey(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("FindByUserKey failed: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("expected 0 permissions, got %d", len(rels))
	}
}

// --- SysUserNodeRepo tests ---

func TestSysUserNodeRepo_CRUD(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewSysUserNodeRepo(db)
	ctx := context.Background()

	err := repo.Create(ctx, &model.SysUserNodeRelDO{UserID: "user-1", NodeID: "alice"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	err = repo.Create(ctx, &model.SysUserNodeRelDO{UserID: "user-1", NodeID: "bob"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	rels, err := repo.FindByUserID(ctx, "user-1")
	if err != nil {
		t.Fatalf("FindByUserID failed: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("expected 2 node relations, got %d", len(rels))
	}

	err = repo.DeleteByUserID(ctx, "user-1")
	if err != nil {
		t.Fatalf("DeleteByUserID failed: %v", err)
	}

	rels, _ = repo.FindByUserID(ctx, "user-1")
	if len(rels) != 0 {
		t.Errorf("expected 0 relations after delete, got %d", len(rels))
	}
}

func TestSysUserNodeRepo_FindByUserID_Empty(t *testing.T) {
	db := setupUserTestDB(t)
	repo := NewSysUserNodeRepo(db)

	rels, err := repo.FindByUserID(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("FindByUserID failed: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("expected 0 relations, got %d", len(rels))
	}
}
