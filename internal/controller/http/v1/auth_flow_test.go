package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type authFlowEnv struct {
	r  *gin.Engine
	db *gorm.DB
}

func newAuthFlowEnv(t *testing.T) *authFlowEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserAccountsDO{}, &model.UserTokensDO{}, &model.ProjectNodeDO{},
		&model.InstDO{}, &model.NodeDO{}, &model.SysResourceDO{}, &model.SysRoleResourceRelDO{},
		&model.SysUserPermissionRelDO{}, &model.SysUserNodeRelDO{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&model.UserAccountsDO{Name: "admin", PasswordHash: service.HashPassword("12345678"), OwnerType: "CENTER", OwnerID: "kuscia-system"})
	db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice"})
	db.Create(&model.UserAccountsDO{Name: "eu", PasswordHash: service.HashPassword("old"), OwnerType: "EDGE", OwnerID: "alice"})

	jwtMgr := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	authSvc := service.NewAuthService(repository.NewUserAccountsRepo(db), repository.NewUserTokensRepo(db), jwtMgr)
	env := service.NewEnvService("master", "kuscia-system", "", nil)
	authSvc.SetEnv(env)
	userSvc := service.NewUserService(repository.NewUserAccountsRepo(db), repository.NewSysUserPermissionRepo(db),
		repository.NewSysUserNodeRepo(db), db)
	nodeUserSvc := service.NewNodeUserService(db)
	middleware.SetDataResourceAuth(service.NewDataResourceAuth(db, env))
	t.Cleanup(func() { middleware.SetDataResourceAuth(nil) })

	ah := NewAuthHandler(authSvc)
	uh := NewUserHandler(userSvc).SetNodeUserService(nodeUserSvc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	authMW := []gin.HandlerFunc{middleware.SessionAuth(jwtMgr, authSvc, true), middleware.RBAC(db, env, middleware.RBACConfig{Enabled: true})}
	r.POST("/api/login", ah.Login)
	r.POST("/api/user/refresh", ah.RefreshToken)
	r.POST("/api/logout", append(authMW, ah.Logout)...)
	g := r.Group("/api/v1alpha1", authMW...)
	g.POST("/user/get", uh.Get)
	g.POST("/user/updatePwd", uh.UpdatePwd)
	g.POST("/user/create", uh.Create)
	g.POST("/user/remote/resetPassword", uh.RemoteResetPassword)

	inner := r.Group("/inner", middleware.InnerAuth(authSvc), middleware.RBAC(db, env, middleware.RBACConfig{Enabled: true}))
	inner.POST("/user/node/resetPassword", uh.NodeResetPassword)
	return &authFlowEnv{r: r, db: db}
}

type flowResp struct {
	Status struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	} `json:"status"`
	Data json.RawMessage `json:"data"`
}

func (e *authFlowEnv) post(t *testing.T, path, token string, body interface{}, hdr ...string) (int, flowResp) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("User-Token", token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	var fr flowResp
	_ = json.Unmarshal(w.Body.Bytes(), &fr)
	return w.Code, fr
}

func TestAuthFlow_LoginGetLogout(t *testing.T) {
	e := newAuthFlowEnv(t)
	_, resp := e.post(t, "/api/login", "", map[string]string{"name": "admin", "passwordHash": service.HashPassword("12345678")})
	if resp.Status.Code != 0 {
		t.Fatalf("login failed: %+v", resp.Status)
	}
	var data map[string]interface{}
	_ = json.Unmarshal(resp.Data, &data)
	for _, k := range []string{"token", "name", "platformType", "platformNodeId", "ownerType", "ownerId", "projectIds", "apiResources", "deployMode", "access_token", "user"} {
		if _, ok := data[k]; !ok {
			t.Errorf("login response missing %q", k)
		}
	}
	token := data["token"].(string)

	_, got := e.post(t, "/api/v1alpha1/user/get", token, map[string]string{"name": "someone-else"})
	var uc map[string]interface{}
	_ = json.Unmarshal(got.Data, &uc)
	if got.Status.Code != 0 || uc["name"] != "admin" || uc["platformType"] != "CENTER" {
		t.Fatalf("user/get must return the session user: %+v %v", got.Status, uc)
	}

	code, out := e.post(t, "/api/logout", token, map[string]string{})
	if code != 200 || out.Status.Code != 0 {
		t.Fatalf("logout failed: %d %+v", code, out.Status)
	}
	if code, _ := e.post(t, "/api/v1alpha1/user/get", token, nil); code != http.StatusUnauthorized {
		t.Fatalf("token must be invalid after logout, got %d", code)
	}
	if code, _ := e.post(t, "/api/logout", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated logout must be 401, got %d", code)
	}
}

func TestAuthFlow_LoginFailureUniform(t *testing.T) {
	e := newAuthFlowEnv(t)
	_, a := e.post(t, "/api/login", "", map[string]string{"name": "admin", "passwordHash": service.HashPassword("bad")})
	_, b := e.post(t, "/api/login", "", map[string]string{"name": "nobody", "passwordHash": service.HashPassword("bad")})
	if a.Status.Code != 202011601 || b.Status.Code != 202011601 {
		t.Fatalf("expected uniform USER_PASSWORD_ERROR, got %d / %d", a.Status.Code, b.Status.Code)
	}
	if a.Status.Msg != b.Status.Msg {
		t.Errorf("messages differ: %q vs %q", a.Status.Msg, b.Status.Msg)
	}
	// Plaintext password is not accepted outside dev.
	_, c := e.post(t, "/api/login", "", map[string]string{"name": "admin", "password": "12345678"})
	if c.Status.Code == 0 {
		t.Error("plaintext login must fail")
	}
}

func TestAuthFlow_UpdatePwdUsesSessionName(t *testing.T) {
	e := newAuthFlowEnv(t)
	_, resp := e.post(t, "/api/login", "", map[string]string{"name": "eu", "passwordHash": service.HashPassword("old")})
	var data map[string]interface{}
	_ = json.Unmarshal(resp.Data, &data)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("login failed: %+v", resp.Status)
	}
	// EDGE user without USER_UPDATE_PWD resource is denied by RBAC (Java aspect).
	_, r := e.post(t, "/api/v1alpha1/user/updatePwd", token, map[string]string{
		"name": "admin", "oldPasswordHash": service.HashPassword("old"),
		"newPasswordHash": service.HashPassword("new"), "confirmPasswordHash": service.HashPassword("new"),
	})
	if r.Status.Code != 202011602 {
		t.Fatalf("expected AUTH_FAILED without resource, got %+v", r.Status)
	}
	// Grant the Java EDGE_USER role and log in again.
	e.db.Create(&model.SysResourceDO{ResourceType: "API", ResourceCode: "USER_UPDATE_PWD", ResourceName: "USER_UPDATE_PWD"})
	e.db.Create(&model.SysRoleResourceRelDO{RoleCode: "EDGE_USER", ResourceCode: "USER_UPDATE_PWD"})
	e.db.Create(&model.SysUserPermissionRelDO{UserType: "EDGE_USER", UserKey: "eu", TargetType: "ROLE", TargetCode: "EDGE_USER"})
	_, resp = e.post(t, "/api/login", "", map[string]string{"name": "eu", "passwordHash": service.HashPassword("old")})
	_ = json.Unmarshal(resp.Data, &data)
	token = data["token"].(string)
	_, r = e.post(t, "/api/v1alpha1/user/updatePwd", token, map[string]string{
		"name": "admin", "oldPasswordHash": service.HashPassword("old"),
		"newPasswordHash": service.HashPassword("new"), "confirmPasswordHash": service.HashPassword("new"),
	})
	if r.Status.Code != 0 || string(r.Data) != "true" {
		t.Fatalf("updatePwd failed: %+v %s", r.Status, r.Data)
	}
	var admin, eu model.UserAccountsDO
	e.db.Where("name = ?", "admin").First(&admin)
	e.db.Where("name = ?", "eu").First(&eu)
	matches := func(stored, plain string) bool {
		return auth.VerifyStoredPassword(stored, auth.PasswordCandidates{SHA256: service.HashPassword(plain)}).OK
	}
	if !matches(admin.PasswordHash, "12345678") || !matches(eu.PasswordHash, "new") {
		t.Error("updatePwd must change the session user's password only")
	}
	// Sessions were cleared.
	if code, _ := e.post(t, "/api/v1alpha1/user/get", token, nil); code != http.StatusUnauthorized {
		t.Errorf("sessions must be cleared after password change, got %d", code)
	}
}

func TestAuthFlow_AdminOnlyAndInnerReset(t *testing.T) {
	e := newAuthFlowEnv(t)
	e.db.Create(&model.SysUserPermissionRelDO{UserType: "USER", UserKey: "admin", TargetType: "ROLE", TargetCode: "ADMIN"})
	middleware.InvalidateUserCache("admin")
	middleware.InvalidateUserCache("eu")
	login := func(name, pwd string) string {
		_, resp := e.post(t, "/api/login", "", map[string]string{"name": name, "passwordHash": service.HashPassword(pwd)})
		var d map[string]interface{}
		_ = json.Unmarshal(resp.Data, &d)
		s, _ := d["token"].(string)
		return s
	}
	adminTok := login("admin", "12345678")
	_, r := e.post(t, "/api/v1alpha1/user/create", adminTok, map[string]string{"name": "n1", "passwordHash": service.HashPassword("p")})
	if r.Status.Code != 0 {
		t.Fatalf("admin create failed: %+v", r.Status)
	}
	euTok := login("eu", "old")
	_, r = e.post(t, "/api/v1alpha1/user/create", euTok, map[string]string{"name": "n2", "passwordHash": service.HashPassword("p")})
	if r.Status.Code != 202011602 {
		t.Fatalf("non-admin create must be denied, got %+v", r.Status)
	}

	// Inner port: node bob cannot reset alice's user; node alice can.
	e.db.Create(&model.SysResourceDO{ResourceType: "API", ResourceCode: "NODE_USER_RESET_PWD", ResourceName: "x"})
	e.db.Create(&model.SysRoleResourceRelDO{RoleCode: "EDGE_NODE", ResourceCode: "NODE_USER_RESET_PWD"})
	for _, n := range []string{"alice", "bob"} {
		e.db.Create(&model.SysUserPermissionRelDO{UserType: "NODE", UserKey: n, TargetType: "ROLE", TargetCode: "EDGE_NODE"})
	}
	body := map[string]string{"nodeId": "alice", "name": "eu", "passwordHash": service.HashPassword("old"), "newPasswordHash": service.HashPassword("x")}
	if code, _ := e.post(t, "/inner/user/node/resetPassword", "", body); code != http.StatusUnauthorized {
		t.Fatalf("missing kuscia-origin-source must be rejected, got %d", code)
	}
	_, r = e.post(t, "/inner/user/node/resetPassword", "", body, "kuscia-origin-source", "bob")
	if r.Status.Code != 202011602 {
		t.Fatalf("foreign node must be denied by DataResource, got %+v", r.Status)
	}
	_, r = e.post(t, "/inner/user/node/resetPassword", "", map[string]string{"nodeId": "alice", "name": "admin",
		"passwordHash": service.HashPassword("12345678"), "newPasswordHash": service.HashPassword("x")}, "kuscia-origin-source", "alice")
	if r.Status.Code == 0 {
		t.Fatal("node must not reset an account of another owner (admin)")
	}
	_, r = e.post(t, "/inner/user/node/resetPassword", "", body, "kuscia-origin-source", "alice")
	if r.Status.Code != 0 {
		t.Fatalf("own node reset failed: %+v", r.Status)
	}

	// Remote reset without gateway configured -> REMOTE_CALL_ERROR.
	_, r = e.post(t, "/api/v1alpha1/user/remote/resetPassword", adminTok, body)
	if r.Status.Code != 2020111011 {
		t.Fatalf("expected REMOTE_CALL_ERROR, got %+v", r.Status)
	}
}
