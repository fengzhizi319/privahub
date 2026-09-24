package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"gorm.io/gorm"
)

// NodeUser service errors.
var (
	ErrNodeUserNotFound     = errors.New("node user not found")
	ErrNodeUserExists       = errors.New("node user already exists")
	ErrNodeNotExists        = errors.New("node does not exist")
	ErrRemoteNotConfigured  = errors.New("kuscia gateway is not configured")
	ErrNodeUserWrongPwd     = errors.New("wrong password")
	ErrNodeUserInvalidParam = errors.New("invalid parameter")
)

// NodeUserService handles node-specific (EDGE) user management
// (Java NodeUserServiceImpl + RemoteUserController).
type NodeUserService struct {
	db *gorm.DB

	// Remote reset proxy (Java RemoteUserController): requests are sent to
	// http://<gateway>/api/v1alpha1/user/node/resetPassword with Host set to
	// the center platform service so Kuscia's gateway routes them to center.
	gateway    string
	centerHost string
	httpClient *http.Client
	// localCenter: this instance IS the center platform (the proxy target of
	// Java RemoteUserController), so a remote reset is served locally instead
	// of looping back through the Kuscia gateway (which may be absent).
	localCenter bool
}

// NewNodeUserService creates a new NodeUserService.
func NewNodeUserService(db *gorm.DB) *NodeUserService {
	return &NodeUserService{db: db, httpClient: &http.Client{Timeout: 15 * time.Second}}
}

// SetRemote configures the Kuscia gateway (host:port) and the center platform
// service Host header used by RemoteResetPassword.
func (s *NodeUserService) SetRemote(gateway, centerHost string) {
	s.gateway = strings.TrimSpace(gateway)
	s.centerHost = strings.TrimSpace(centerHost)
}

// SetLocalCenter marks this instance as the center platform: RemoteResetPassword
// then resets locally (same effect as Java's gateway loop-back to its own
// NodeUserController) and does not require a Kuscia gateway.
func (s *NodeUserService) SetLocalCenter(v bool) { s.localCenter = v }

// --- DTOs ---

// NodeUserCreateRequest represents a node user creation request (Java
// NodeUserCreateRequest{nodeId,name,passwordHash}); snake_case / legacy field
// names are accepted as fallbacks.
type NodeUserCreateRequest struct {
	NodeID       string `json:"node_id"`
	NodeIDAlt    string `json:"nodeId"`
	Name         string `json:"name"`
	UserName     string `json:"user_name"`
	UserNameAlt  string `json:"userName"`
	PasswordHash string `json:"passwordHash"`
	Password     string `json:"password"`
}

// ResetNodeUserPwdRequest is the Java ResetNodeUserPwdRequest: the account
// `name` must belong to `nodeId` and `passwordHash` must match the current
// password before `newPasswordHash` is stored.
type ResetNodeUserPwdRequest struct {
	NodeID          string `json:"nodeId"`
	Name            string `json:"name"`
	PasswordHash    string `json:"passwordHash"`
	NewPasswordHash string `json:"newPasswordHash"`

	// Legacy Go aliases.
	NodeIDSnake string `json:"node_id,omitempty"`
	UserName    string `json:"user_name,omitempty"`
	UserNameAlt string `json:"userName,omitempty"`
}

func (r *ResetNodeUserPwdRequest) normalize() {
	if r.NodeID == "" {
		r.NodeID = r.NodeIDSnake
	}
	if r.Name == "" {
		r.Name = r.UserName
	}
	if r.Name == "" {
		r.Name = r.UserNameAlt
	}
}

// Validate checks required fields.
func (r *ResetNodeUserPwdRequest) Validate() error {
	r.normalize()
	if r.NodeID == "" || r.Name == "" || r.PasswordHash == "" || r.NewPasswordHash == "" {
		return ErrNodeUserInvalidParam
	}
	return nil
}

// NodeUserListRequest represents a node user list request.
type NodeUserListRequest struct {
	NodeID    string `json:"node_id"`
	NodeIDAlt string `json:"nodeId"`
}

// NodeUserVO represents a node user view object.
type NodeUserVO struct {
	Name      string `json:"name"`
	NodeID    string `json:"node_id"`
	GmtCreate string `json:"gmt_create"`
}

// --- Service Methods ---

// Create creates a new node (EDGE) user: the node must exist, the name must
// be globally unique, and the account gets the EDGE_USER role (user_type
// EDGE_USER) like Java NodeUserServiceImpl.create.
func (s *NodeUserService) Create(ctx context.Context, req *NodeUserCreateRequest) error {
	if req.NodeID == "" {
		req.NodeID = req.NodeIDAlt
	}
	if req.Name == "" {
		req.Name = req.UserName
	}
	if req.Name == "" {
		req.Name = req.UserNameAlt
	}
	req.UserName = req.Name
	pwd := clientPasswordHash(req.PasswordHash, req.Password)
	if req.NodeID == "" || req.Name == "" || pwd == "" {
		return ErrNodeUserInvalidParam
	}

	var nodeCount int64
	if err := s.db.WithContext(ctx).Model(&model.NodeDO{}).
		Where("node_id = ?", req.NodeID).Count(&nodeCount).Error; err != nil {
		return err
	}
	if nodeCount == 0 {
		return ErrNodeNotExists
	}

	// Global name uniqueness (Java findByName).
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.UserAccountsDO{}).
		Where("name = ?", req.Name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrNodeUserExists
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user := &model.UserAccountsDO{
			Name:         req.Name,
			PasswordHash: pwd,
			OwnerType:    auth.OwnerTypeEdge,
			OwnerID:      req.NodeID,
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.SysUserPermissionRelDO{
			UserType:   auth.PermissionUserTypeEdgeUser,
			UserKey:    req.Name,
			TargetType: "ROLE",
			TargetCode: auth.RoleEdgeUser,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.SysUserNodeRelDO{UserID: req.Name, NodeID: req.NodeID}).Error
	})
}

// ResetPassword implements Java NodeUserServiceImpl.resetPassword: the account
// must exist and belong to nodeId, and the old hash must match.
func (s *NodeUserService) ResetPassword(ctx context.Context, req *ResetNodeUserPwdRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	var user model.UserAccountsDO
	if err := s.db.WithContext(ctx).Where("name = ?", req.Name).First(&user).Error; err != nil {
		return ErrNodeUserNotFound
	}
	if user.OwnerID != req.NodeID {
		return ErrNodeUserNotFound
	}
	if !constEq(strings.ToLower(user.PasswordHash), strings.ToLower(req.PasswordHash)) {
		return ErrNodeUserWrongPwd
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password_hash", strings.ToLower(req.NewPasswordHash)).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("name = ?", user.Name).Delete(&model.UserTokensDO{}).Error
	})
}

// NodeUserErrorCode maps NodeUserService errors to Java error codes + args.
func NodeUserErrorCode(err error, req interface{}) (*errcode.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrNodeNotExists):
		return errcode.SystemValidationError, "Cancel create user. Invalid nodeId."
	case errors.Is(err, ErrNodeUserExists):
		return errcode.SystemValidationError, "Cancel create account. The name already exists in db."
	case errors.Is(err, ErrNodeUserNotFound):
		if r, ok := req.(*ResetNodeUserPwdRequest); ok {
			return errcode.SystemValidationError, fmt.Sprintf("Cancel reset password. Do not exists name(%s) for nodeId(%s) in db.", r.Name, r.NodeID)
		}
		return errcode.SystemValidationError, "user not found"
	case errors.Is(err, ErrNodeUserWrongPwd):
		return errcode.SystemValidationError, "wrong password"
	case errors.Is(err, ErrNodeUserInvalidParam):
		return errcode.SystemValidationError, "nodeId/name/passwordHash/newPasswordHash required"
	case errors.Is(err, ErrRemoteNotConfigured):
		return errcode.SystemRemoteCallError, err.Error()
	}
	return errcode.SystemUnknownError, err.Error()
}

// RemoteResetPassword proxies a node-user password reset to the center
// platform through the Kuscia gateway (Java RemoteUserController.resetPwd) and
// returns the raw JSON response of the center.
func (s *NodeUserService) RemoteResetPassword(ctx context.Context, req *ResetNodeUserPwdRequest) (json.RawMessage, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if s.localCenter {
		if err := s.ResetPassword(ctx, req); err != nil {
			return nil, err
		}
		return json.RawMessage(`{"status":{"code":0,"msg":"success"},"data":"ok"}`), nil
	}
	if s.gateway == "" {
		return nil, ErrRemoteNotConfigured
	}
	gw := s.gateway
	if !strings.HasPrefix(gw, "http://") && !strings.HasPrefix(gw, "https://") {
		gw = "http://" + gw
	}
	body, _ := json.Marshal(map[string]string{
		"nodeId":          req.NodeID,
		"name":            req.Name,
		"passwordHash":    req.PasswordHash,
		"newPasswordHash": req.NewPasswordHash,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(gw, "/")+"/api/v1alpha1/user/node/resetPassword", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if s.centerHost != "" {
		httpReq.Host = s.centerHost
	}
	innerauth.Apply(httpReq)
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("remote reset password: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || !json.Valid(raw) {
		return nil, fmt.Errorf("remote reset password: HTTP %d: %s", resp.StatusCode, truncateForLog(string(raw), 200))
	}
	return raw, nil
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ListByNodeId lists all users for a specific node.
func (s *NodeUserService) ListByNodeId(ctx context.Context, req *NodeUserListRequest) ([]NodeUserVO, error) {
	if req.NodeID == "" {
		req.NodeID = req.NodeIDAlt
	}
	var users []model.UserAccountsDO
	if err := s.db.WithContext(ctx).
		Where("owner_id = ?", req.NodeID).
		Order("gmt_create DESC").
		Find(&users).Error; err != nil {
		return nil, err
	}

	result := make([]NodeUserVO, 0, len(users))
	for _, u := range users {
		result = append(result, NodeUserVO{
			Name:      u.Name,
			NodeID:    req.NodeID,
			GmtCreate: u.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}
