package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"gorm.io/gorm"
)

// seedNodeUserTestNodes creates the alice/bob nodes required by node users.
func seedNodeUserTestNodes(db *gorm.DB) {
	for _, id := range []string{"alice", "bob"} {
		var n int64
		db.Model(&model.NodeDO{}).Where("node_id = ?", id).Count(&n)
		if n == 0 {
			db.Create(&model.NodeDO{NodeID: id, Name: id, ControlNodeID: id})
		}
	}
}

func newPwdTestUserService(t *testing.T) (*UserService, *gorm.DB) {
	t.Helper()
	db := setupAuthExtTestDB(t)
	_ = db.AutoMigrate(&model.SysUserNodeRelDO{})
	svc := NewUserService(repository.NewUserAccountsRepo(db), repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db), db)
	return svc, db
}

func TestUserService_UpdatePassword_Java(t *testing.T) {
	svc, db := newPwdTestUserService(t)
	svc.SetResetPasswordPolicy(2, 30*time.Minute)
	createUser(t, db, "u1", "old", "CENTER", "kuscia-system")
	db.Create(&model.UserTokensDO{Name: "u1", Token: "t1"})
	ctx := context.Background()
	oldH, newH := HashPassword("old"), HashPassword("new")

	cases := []struct {
		req  UpdatePasswordRequest
		code int
	}{
		{UpdatePasswordRequest{Name: "u1", OldPasswordHash: oldH, NewPasswordHash: newH, ConfirmPasswordHash: "x"}, errcode.UserUpdatePasswordErrorInconsistent.Code},
		{UpdatePasswordRequest{Name: "u1", OldPasswordHash: oldH, NewPasswordHash: oldH, ConfirmPasswordHash: oldH}, errcode.UserUpdatePasswordErrorSame.Code},
		{UpdatePasswordRequest{Name: "u1", OldPasswordHash: "", NewPasswordHash: newH, ConfirmPasswordHash: newH}, errcode.SystemValidationError.Code},
	}
	for i, c := range cases {
		req := c.req
		if got := authCode(svc.UpdatePassword(ctx, &req)); got != c.code {
			t.Errorf("case %d: code %d, want %d", i, got, c.code)
		}
	}

	// Wrong old password counts and locks after 2 attempts.
	bad := UpdatePasswordRequest{Name: "u1", OldPasswordHash: HashPassword("bad"), NewPasswordHash: newH, ConfirmPasswordHash: newH}
	if got := authCode(svc.UpdatePassword(ctx, &bad)); got != errcode.AuthUserPasswordError.Code {
		t.Errorf("first wrong old pwd: %d", got)
	}
	if got := authCode(svc.UpdatePassword(ctx, &bad)); got != errcode.AuthResetPasswordIsLocked.Code {
		t.Errorf("second wrong old pwd should lock: %d", got)
	}
	good := UpdatePasswordRequest{Name: "u1", OldPasswordHash: oldH, NewPasswordHash: newH, ConfirmPasswordHash: newH}
	if got := authCode(svc.UpdatePassword(ctx, &good)); got != errcode.AuthUserIsLocked.Code {
		t.Errorf("locked updatePwd should fail: %d", got)
	}
	// After the lock expires the change succeeds and sessions are cleared.
	svc.now = func() time.Time { return time.Now().Add(31 * time.Minute) }
	if err := svc.UpdatePassword(ctx, &good); err != nil {
		t.Fatalf("update after lock expiry failed: %v", err)
	}
	var u model.UserAccountsDO
	db.Where("name = ?", "u1").First(&u)
	if !storedMatches(u.PasswordHash, "new") || u.PasswdResetFailedAttempts != nil || u.PasswordVersion != 1 {
		t.Errorf("password not updated / counters not reset: %+v", u)
	}
	var n int64
	db.Model(&model.UserTokensDO{}).Where("name = ?", "u1").Count(&n)
	if n != 0 {
		t.Error("sessions must be cleared after password change")
	}
}

func TestNodeUserService_Create_JavaRules(t *testing.T) {
	db := setupAuthExtTestDB(t)
	_ = db.AutoMigrate(&model.SysUserNodeRelDO{})
	seedNodeUserTestNodes(db)
	svc := NewNodeUserService(db)
	ctx := context.Background()

	if err := svc.Create(ctx, &NodeUserCreateRequest{NodeIDAlt: "nope", Name: "x", PasswordHash: HashPassword("p")}); !errors.Is(err, ErrNodeNotExists) {
		t.Fatalf("unknown node should be rejected: %v", err)
	}
	createUser(t, db, "taken", "p", "CENTER", "kuscia-system")
	if err := svc.Create(ctx, &NodeUserCreateRequest{NodeIDAlt: "alice", Name: "taken", PasswordHash: HashPassword("p")}); !errors.Is(err, ErrNodeUserExists) {
		t.Fatalf("name must be globally unique: %v", err)
	}
	if err := svc.Create(ctx, &NodeUserCreateRequest{NodeIDAlt: "alice", Name: "eu", PasswordHash: HashPassword("p")}); err != nil {
		t.Fatal(err)
	}
	var rel model.SysUserPermissionRelDO
	if err := db.Where("user_key = ? AND target_code = ?", "eu", "EDGE_USER").First(&rel).Error; err != nil {
		t.Fatalf("EDGE_USER role not granted: %v", err)
	}
	if rel.UserType != "EDGE_USER" {
		t.Errorf("user_type = %s", rel.UserType)
	}
	var u model.UserAccountsDO
	db.Where("name = ?", "eu").First(&u)
	if !storedMatches(u.PasswordHash, "p") || u.OwnerType != "EDGE" || u.OwnerID != "alice" {
		t.Errorf("unexpected account %+v", u)
	}
}

func TestNodeUserService_RemoteResetPassword(t *testing.T) {
	svc := NewNodeUserService(nil)
	req := &ResetNodeUserPwdRequest{NodeID: "alice", Name: "eu", PasswordHash: "a", NewPasswordHash: "b"}
	if _, err := svc.RemoteResetPassword(context.Background(), req); !errors.Is(err, ErrRemoteNotConfigured) {
		t.Fatalf("expected not configured, got %v", err)
	}

	var gotHost, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost, gotPath = r.Host, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"status":{"code":0,"msg":"success"},"data":"ok"}`))
	}))
	defer srv.Close()
	svc.SetRemote(strings.TrimPrefix(srv.URL, "http://"), "secretpad.master.svc")
	raw, err := svc.RemoteResetPassword(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if gotHost != "secretpad.master.svc" || gotPath != "/api/v1alpha1/user/node/resetPassword" {
		t.Errorf("host=%s path=%s", gotHost, gotPath)
	}
	if gotBody["nodeId"] != "alice" || gotBody["newPasswordHash"] != "b" {
		t.Errorf("body = %v", gotBody)
	}
	if !strings.Contains(string(raw), `"data":"ok"`) {
		t.Errorf("raw = %s", raw)
	}
}

func TestDataResourceAuth(t *testing.T) {
	db := setupAuthExtTestDB(t)
	_ = db.AutoMigrate(&model.ProjectDO{}, &model.ProjectInstDO{})
	db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice", InstID: "instA"})
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	db.Create(&model.ProjectDO{ProjectID: "p1", Name: "p1"})
	db.Create(&model.ProjectDO{ProjectID: "p2", Name: "p2"})
	ctx := context.Background()

	center := NewDataResourceAuth(db, NewEnvService("CENTER", "kuscia-system", "", nil))
	admin := &auth.UserContext{Name: "admin", OwnerType: "CENTER", OwnerID: "kuscia-system"}
	edge := &auth.UserContext{Name: "eu", OwnerType: "EDGE", OwnerID: "alice"}
	p2p := &auth.UserContext{Name: "p", OwnerType: "P2P", OwnerID: "instA"}

	if !center.Check(ctx, admin, DataResourceNodeID, "bob") {
		t.Error("center user can access everything")
	}
	if !center.Check(ctx, edge, DataResourceNodeID, "alice") || center.Check(ctx, edge, DataResourceNodeID, "bob") {
		t.Error("edge user node check wrong")
	}
	if !center.Check(ctx, edge, DataResourceProjectID, "p1") || center.Check(ctx, edge, DataResourceProjectID, "p2") {
		t.Error("edge user project check wrong")
	}
	if !center.Check(ctx, p2p, DataResourceNodeID, "alice") {
		t.Error("inst owner should access its nodes")
	}
	if center.Check(ctx, nil, DataResourceNodeID, "alice") {
		t.Error("nil user denied")
	}
	edgePlatform := NewDataResourceAuth(db, NewEnvService("EDGE", "alice", "", nil))
	if !edgePlatform.Check(ctx, edge, DataResourceProjectID, "p2") {
		t.Error("edge platform skips non-node checks")
	}
	if edgePlatform.Check(ctx, edge, DataResourceNodeID, "bob") {
		t.Error("edge platform still checks NODE_ID")
	}
}
