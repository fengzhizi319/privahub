// Package service provides business logic services for Privahub.
//
// This package implements the core domain logic including:
//   - Authentication & session management (AuthService)
//   - Project/graph/job lifecycle management
//   - Node and route management with Kuscia integration
//   - Datatable and datasource operations
//   - Vote/approval workflows for cross-institution collaboration
//   - Background sync services for Kuscia state reconciliation
//
// All services follow the constructor injection pattern and accept
// repository interfaces for testability.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Common errors for auth service. Errors returned by Login/UpdatePwd are
// *AuthError values that unwrap to one of these sentinels, so errors.Is keeps
// working while the handler can render the Java error code + message args.
var (
	ErrUserNotFound     = errors.New("user not found")
	ErrInvalidPassword  = errors.New("invalid password")
	ErrUserLocked       = errors.New("user account locked")
	ErrTokenNotFound    = errors.New("token not found")
	ErrSessionExpired   = errors.New("session expired")
	ErrPasswordRequired = errors.New("password hash required")
)

// AuthError carries a Java error code and its message arguments.
type AuthError struct {
	Code *errcode.ErrorCode
	Args []string
	err  error
}

func (e *AuthError) Error() string { return e.err.Error() }
func (e *AuthError) Unwrap() error { return e.err }

func newAuthError(sentinel error, code *errcode.ErrorCode, args ...string) *AuthError {
	return &AuthError{Code: code, Args: args, err: sentinel}
}

// AuthOptions configures AuthService behaviour (see config.AuthConfig).
type AuthOptions struct {
	// MaxAttempts is the number of failed logins before locking (default 5).
	MaxAttempts int
	// LockDuration is the lock period after MaxAttempts failures (default 30m).
	LockDuration time.Duration
	// SessionIdleTimeout is the sliding session expiry (default 24h, Java EXPIRE).
	SessionIdleTimeout time.Duration
	// AllowPlaintextPassword accepts a plaintext `password` (server hashes it).
	// Only enabled for dev/test profiles.
	AllowPlaintextPassword bool
	// DeployMode is reported in the UserContext (MPC | TEE | ALL-IN-ONE).
	DeployMode string
}

func (o *AuthOptions) withDefaults() {
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
	if o.LockDuration <= 0 {
		o.LockDuration = 30 * time.Minute
	}
	if o.SessionIdleTimeout <= 0 {
		o.SessionIdleTimeout = 24 * time.Hour
	}
	if o.DeployMode == "" {
		o.DeployMode = "ALL-IN-ONE"
	}
}

// AuthService handles user authentication and session management.
//
// Password contract (documented for the frontend):
//
//	POST /api/login {name, passwordHash, passwordHashSm3?}
//	  passwordHash    = lowercase hex SHA-256(password)   — the standard scheme,
//	                    stored as user_accounts.password_hash.
//	  passwordHashSm3 = lowercase hex SM3(password)       — optional; only used for
//	                    accounts migrated from Java SecretPad, whose DbDataInit and
//	                    frontend stored/sent SM3(password). If the stored hash equals
//	                    passwordHashSm3, login succeeds and the stored hash is
//	                    upgraded in place to passwordHash (SHA-256).
//	  password        = accepted as an alias of passwordHash (the v1alpha1 route of
//	                    the new frontend sends the SHA-256 hex in this field). Only in
//	                    dev/test profiles is a raw plaintext password also hashed and
//	                    compared server side.
//
// Sessions: the access token (JWT) is stored in user_tokens together with the
// UserContext snapshot (session_data). Every authenticated request must match a
// user_tokens row whose gmt_token is within SessionIdleTimeout (sliding, 24h by
// default like Java LoginInterceptor); logout deletes the row.
type AuthService struct {
	userRepo   repository.UserAccountsRepository
	tokenRepo  repository.UserTokensRepository
	jwtManager *auth.JWTManager
	db         *gorm.DB
	env        *EnvService
	opts       AuthOptions

	// failCache counts failed logins for non-existent users (Java USER_LOCK_CACHE).
	failMu    sync.Mutex
	failCache map[string]*failEntry

	now func() time.Time
}

type failEntry struct {
	count   int
	expires time.Time
}

// NewAuthService creates a new AuthService with default options.
func NewAuthService(
	userRepo repository.UserAccountsRepository,
	tokenRepo repository.UserTokensRepository,
	jwtManager *auth.JWTManager,
) *AuthService {
	s := &AuthService{
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		jwtManager: jwtManager,
		failCache:  make(map[string]*failEntry),
		now:        time.Now,
	}
	if dber, ok := tokenRepo.(interface{ DB() *gorm.DB }); ok {
		s.db = dber.DB()
	}
	s.opts.withDefaults()
	return s
}

// SetOptions replaces the service options (zero values fall back to defaults).
func (s *AuthService) SetOptions(opts AuthOptions) {
	opts.withDefaults()
	s.opts = opts
}

// Options returns the effective options.
func (s *AuthService) Options() AuthOptions { return s.opts }

// SetEnv injects the EnvService (platform type / node id for UserContext).
func (s *AuthService) SetEnv(env *EnvService) { s.env = env }

// SetDB overrides the database handle used for session/resource queries.
func (s *AuthService) SetDB(db *gorm.DB) { s.db = db }

// JWTManager returns the JWT manager.
func (s *AuthService) JWTManager() *auth.JWTManager { return s.jwtManager }

// LoginRequest represents a login request (see AuthService doc for the contract).
type LoginRequest struct {
	Username string `json:"name" binding:"required"`
	// Password is accepted as an alias of PasswordHash; plaintext only in dev.
	Password string `json:"password"`
	// PasswordHash is the SHA-256 hex of the password.
	PasswordHash string `json:"passwordHash"`
	// PasswordHashSm3 is the SM3 hex of the password (legacy Java accounts).
	PasswordHashSm3 string `json:"passwordHashSm3"`
}

// LoginResponse is the Java UserContextDTO plus the Go token pair fields
// (access_token/refresh_token/expires_at/token_type) and a legacy `user`
// object kept for older frontend builds.
type LoginResponse struct {
	auth.UserContext
	*auth.TokenPair
	User *UserInfo `json:"user"`
}

// UserInfo represents basic user information.
type UserInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	OwnerType string `json:"owner_type"`
	OwnerID   string `json:"owner_id"`
}

var hexHash = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// Login authenticates a user and returns the session UserContext + tokens.
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	name := strings.TrimSpace(req.Username)
	provided := req.PasswordHash
	if provided == "" {
		provided = req.Password
	}
	if name == "" || (provided == "" && req.PasswordHashSm3 == "") {
		return nil, newAuthError(ErrPasswordRequired, errcode.SystemValidationError, "name/passwordHash required")
	}

	user, err := s.userRepo.FindByName(ctx, name)
	if err != nil || user == nil {
		return nil, s.failUnknownUser(name)
	}

	now := s.now()
	// Lock check; an expired lock resets the failure counter.
	if user.LockedInvalidTime != nil {
		if user.LockedInvalidTime.After(now) {
			return nil, newAuthError(ErrUserLocked, errcode.AuthUserIsLocked, minutesLeft(*user.LockedInvalidTime, now))
		}
		user.LockedInvalidTime = nil
		user.FailedAttempts = nil
		_ = s.userRepo.Update(ctx, user)
	}

	matched, legacy := s.matchPassword(provided, req.Password, req.PasswordHashSm3, user.PasswordHash)
	if !matched {
		return nil, s.failKnownUser(ctx, user)
	}

	// Successful login: reset counters, upgrade legacy SM3 hash if applicable.
	if user.FailedAttempts != nil || user.LockedInvalidTime != nil {
		user.FailedAttempts = nil
		user.LockedInvalidTime = nil
		_ = s.userRepo.Update(ctx, user)
	}
	if legacy && hexHash.MatchString(req.PasswordHash) {
		newHash := strings.ToLower(req.PasswordHash)
		if err := s.userRepo.UpdatePassword(ctx, user.Name, newHash); err == nil {
			user.PasswordHash = newHash
		}
	}

	uc := s.BuildUserContext(ctx, user)
	sid := uuid.New().String()
	uc.SessionID = sid
	tokenPair, err := s.jwtManager.GenerateTokenPairWithSession(
		strconv.FormatInt(user.ID, 10), user.Name, user.OwnerType, user.OwnerID, sid)
	if err != nil {
		return nil, err
	}
	uc.Token = tokenPair.AccessToken
	if err := s.storeSession(ctx, uc); err != nil {
		return nil, err
	}

	return &LoginResponse{
		UserContext: *uc,
		TokenPair:   tokenPair,
		User: &UserInfo{
			ID:        user.ID,
			Name:      user.Name,
			OwnerType: user.OwnerType,
			OwnerID:   user.OwnerID,
		},
	}, nil
}

// matchPassword returns (matched, matchedViaLegacySm3).
func (s *AuthService) matchPassword(hash, rawPassword, sm3Hash, stored string) (bool, bool) {
	if stored == "" {
		return false, false
	}
	if hash != "" && constEq(strings.ToLower(hash), strings.ToLower(stored)) {
		return true, false
	}
	if sm3Hash != "" && constEq(strings.ToLower(sm3Hash), strings.ToLower(stored)) {
		return true, true
	}
	if s.opts.AllowPlaintextPassword && rawPassword != "" && constEq(HashPassword(rawPassword), strings.ToLower(stored)) {
		return true, false
	}
	return false, false
}

func constEq(a, b string) bool { return hmac.Equal([]byte(a), []byte(b)) }

func minutesLeft(until, now time.Time) string {
	m := int(until.Sub(now).Minutes() + 0.999)
	if m < 1 {
		m = 1
	}
	return strconv.Itoa(m)
}

// failUnknownUser counts failures for non-existent accounts in memory so that
// the response is indistinguishable from a wrong password (no enumeration).
func (s *AuthService) failUnknownUser(name string) error {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	now := s.now()
	e := s.failCache[name]
	if e == nil || now.After(e.expires) {
		e = &failEntry{}
		s.failCache[name] = e
	}
	if e.count >= s.opts.MaxAttempts {
		return newAuthError(ErrUserLocked, errcode.AuthUserIsLocked, minutesLeft(e.expires, now))
	}
	e.count++
	e.expires = now.Add(s.opts.LockDuration)
	if e.count >= s.opts.MaxAttempts {
		return newAuthError(ErrUserLocked, errcode.AuthUserIsLocked, strconv.Itoa(int(s.opts.LockDuration.Minutes())))
	}
	// Opportunistic cleanup of expired entries.
	if len(s.failCache) > 10000 {
		for k, v := range s.failCache {
			if now.After(v.expires) {
				delete(s.failCache, k)
			}
		}
	}
	return newAuthError(ErrInvalidPassword, errcode.AuthUserPasswordError, strconv.Itoa(s.opts.MaxAttempts-e.count))
}

func (s *AuthService) failKnownUser(ctx context.Context, user *model.UserAccountsDO) error {
	attempts := 1
	if user.FailedAttempts != nil {
		attempts = *user.FailedAttempts + 1
	}
	user.FailedAttempts = &attempts
	if attempts >= s.opts.MaxAttempts {
		lock := s.now().Add(s.opts.LockDuration)
		user.LockedInvalidTime = &lock
		_ = s.userRepo.Update(ctx, user)
		return newAuthError(ErrUserLocked, errcode.AuthUserIsLocked, strconv.Itoa(int(s.opts.LockDuration.Minutes())))
	}
	_ = s.userRepo.Update(ctx, user)
	return newAuthError(ErrInvalidPassword, errcode.AuthUserPasswordError, strconv.Itoa(s.opts.MaxAttempts-attempts))
}

// BuildUserContext builds the Java UserContextDTO for an account (without token).
func (s *AuthService) BuildUserContext(ctx context.Context, user *model.UserAccountsDO) *auth.UserContext {
	uc := &auth.UserContext{
		Name:       user.Name,
		OwnerType:  user.OwnerType,
		OwnerID:    user.OwnerID,
		UserID:     strconv.FormatInt(user.ID, 10),
		DeployMode: s.opts.DeployMode,
	}
	if s.env != nil {
		uc.PlatformType = s.env.PlatformTypeString()
		uc.PlatformNodeID = s.env.GetPlatformNodeId()
		if d := s.env.GetDeployMode(); d != "" {
			uc.DeployMode = d
		}
	} else {
		uc.PlatformType = auth.PlatformTypeCenter
	}
	if s.db != nil {
		if user.OwnerType == auth.OwnerTypeEdge {
			uc.ProjectIDs = s.projectIDsOfNode(ctx, user.OwnerID)
			uc.APIResources = QueryAPIResourceCodes(ctx, s.db, auth.PermissionUserTypeEdgeUser, user.Name)
		}
		uc.OwnerName = s.ownerName(ctx, user.OwnerID)
	}
	return uc.Normalize()
}

// BuildNodeVirtualUser builds the inner-port RPC identity for a calling node
// (Java LoginInterceptor.processByNodeRpcRequest).
func (s *AuthService) BuildNodeVirtualUser(ctx context.Context, nodeID string) *auth.UserContext {
	uc := &auth.UserContext{
		Token:              "token",
		Name:               nodeID,
		OwnerID:            nodeID,
		OwnerType:          auth.OwnerTypeEdge,
		PlatformType:       auth.PlatformTypeEdge,
		VirtualUserForNode: auth.BoolPtr(true),
		DeployMode:         s.opts.DeployMode,
	}
	if s.env != nil {
		uc.PlatformNodeID = s.env.GetPlatformNodeId()
		if d := s.env.GetDeployMode(); d != "" {
			uc.DeployMode = d
		}
	}
	if s.db != nil {
		uc.ProjectIDs = s.projectIDsOfNode(ctx, nodeID)
		uc.APIResources = QueryAPIResourceCodes(ctx, s.db, auth.PermissionUserTypeNode, nodeID)
	}
	return uc.Normalize()
}

// DisabledAuthUser returns the temporary admin injected when auth.enabled=false
// (Java LoginInterceptor.createTmpUserForPlatformType).
func (s *AuthService) DisabledAuthUser() *auth.UserContext {
	uc := &auth.UserContext{Name: "admin", Token: "token", DeployMode: s.opts.DeployMode}
	platform := auth.PlatformTypeCenter
	nodeID := "kuscia-system"
	if s.env != nil {
		platform = s.env.PlatformTypeString()
		nodeID = s.env.GetPlatformNodeId()
	}
	uc.PlatformType = platform
	uc.PlatformNodeID = nodeID
	if platform == auth.PlatformTypeCenter {
		uc.OwnerType = auth.OwnerTypeCenter
		uc.OwnerID = "kuscia-system"
	} else {
		uc.OwnerType = auth.OwnerTypeEdge
		uc.OwnerID = nodeID
	}
	return uc.Normalize()
}

func (s *AuthService) projectIDsOfNode(ctx context.Context, nodeID string) []string {
	var ids []string
	_ = s.db.WithContext(ctx).Model(&model.ProjectNodeDO{}).
		Where("node_id = ?", nodeID).Distinct().Pluck("project_id", &ids).Error
	sort.Strings(ids)
	return ids
}

func (s *AuthService) ownerName(ctx context.Context, ownerID string) string {
	var inst model.InstDO
	if err := s.db.WithContext(ctx).Where("inst_id = ?", ownerID).First(&inst).Error; err == nil {
		return inst.Name
	}
	var node model.NodeDO
	if err := s.db.WithContext(ctx).Where("node_id = ?", ownerID).First(&node).Error; err == nil {
		return node.Name
	}
	return ""
}

// QueryAPIResourceCodes returns the API resource codes granted to a principal
// through its roles (Java SysResourcesBizService.queryResourceCodeByUsername).
func QueryAPIResourceCodes(ctx context.Context, db *gorm.DB, userType, userKey string) []string {
	var roles []string
	if err := db.WithContext(ctx).Model(&model.SysUserPermissionRelDO{}).
		Where("user_key = ? AND user_type = ? AND target_type = ?", userKey, userType, "ROLE").
		Pluck("target_code", &roles).Error; err != nil || len(roles) == 0 {
		return []string{}
	}
	var codes []string
	if err := db.WithContext(ctx).Table("sys_role_resource_rel AS rr").
		Joins("JOIN sys_resource r ON r.resource_code = rr.resource_code").
		Where("rr.role_code IN ? AND r.resource_type = ? AND r.deleted_at IS NULL", roles, "API").
		Distinct().Pluck("rr.resource_code", &codes).Error; err != nil {
		return []string{}
	}
	sort.Strings(codes)
	return codes
}

// Logout deletes the current session (only the given token), like Java
// userTokensRepository.deleteByNameAndToken.
func (s *AuthService) Logout(ctx context.Context, username, token string) error {
	if s.db == nil {
		return s.tokenRepo.DeleteByName(ctx, username)
	}
	return s.db.WithContext(ctx).Unscoped().
		Where("name = ? AND token = ?", username, token).
		Delete(&model.UserTokensDO{}).Error
}

// ClearUserSessions deletes all sessions of a user (after password change).
func (s *AuthService) ClearUserSessions(ctx context.Context, username string) error {
	return ClearUserSessions(ctx, s.db, username)
}

// ClearUserSessions deletes all user_tokens rows of a user.
func ClearUserSessions(ctx context.Context, db *gorm.DB, username string) error {
	if db == nil {
		return nil
	}
	return db.WithContext(ctx).Unscoped().Where("name = ?", username).Delete(&model.UserTokensDO{}).Error
}

// RefreshToken issues a new token pair for a still-valid session. The refresh
// token must belong to an existing session (logout invalidates it).
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	claims, err := s.jwtManager.ValidateToken(refreshToken)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != auth.RefreshToken {
		return nil, auth.ErrInvalidToken
	}
	if s.db == nil || claims.SessionID == "" {
		return nil, ErrTokenNotFound
	}
	var row model.UserTokensDO
	if err := s.db.WithContext(ctx).
		Where("name = ? AND session_id = ?", claims.Username, claims.SessionID).
		First(&row).Error; err != nil {
		return nil, ErrTokenNotFound
	}
	if row.GmtToken != nil && s.now().Sub(*row.GmtToken) > s.opts.SessionIdleTimeout {
		_ = s.db.WithContext(ctx).Unscoped().Delete(&row).Error
		return nil, ErrSessionExpired
	}
	pair, err := s.jwtManager.RefreshAccessToken(refreshToken)
	if err != nil {
		return nil, err
	}
	now := s.now()
	updates := map[string]interface{}{"token": pair.AccessToken, "gmt_token": &now}
	if uc, err := auth.UserContextFromJSON(row.SessionData); err == nil {
		uc.Token = pair.AccessToken
		updates["session_data"] = uc.ToJSON()
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return nil, err
	}
	return pair, nil
}

// ValidateSession checks token against user_tokens with sliding expiry and
// returns the session UserContext (Java LoginInterceptor.processByUserRequest).
func (s *AuthService) ValidateSession(ctx context.Context, token string) (*auth.UserContext, error) {
	if token == "" {
		return nil, ErrTokenNotFound
	}
	row, err := s.tokenRepo.FindByToken(ctx, token)
	if err != nil || row == nil {
		return nil, ErrTokenNotFound
	}
	now := s.now()
	if row.GmtToken == nil || now.Sub(*row.GmtToken) > s.opts.SessionIdleTimeout {
		if s.db != nil {
			_ = s.db.WithContext(ctx).Unscoped().Delete(row).Error
		}
		return nil, ErrSessionExpired
	}
	// Slide the expiry; throttle writes to at most one per minute per session.
	if now.Sub(*row.GmtToken) > time.Minute && s.db != nil {
		_ = s.db.WithContext(ctx).Model(&model.UserTokensDO{}).
			Where("id = ?", row.ID).Update("gmt_token", now).Error
	}
	if strings.TrimSpace(row.SessionData) == "" {
		return nil, ErrTokenNotFound
	}
	uc, err := auth.UserContextFromJSON(row.SessionData)
	if err != nil {
		return nil, ErrTokenNotFound
	}
	uc.Token = token
	uc.SessionID = row.SessionID
	return uc, nil
}

// storeSession saves the session row.
func (s *AuthService) storeSession(ctx context.Context, uc *auth.UserContext) error {
	now := s.now()
	return s.tokenRepo.Create(ctx, &model.UserTokensDO{
		Name:        uc.Name,
		Token:       uc.Token,
		GmtToken:    &now,
		SessionData: uc.ToJSON(),
		SessionID:   uc.SessionID,
	})
}

// verifyPassword checks a client-supplied credential against the stored hash:
// direct (constant-time) comparison of the SHA-256 hex, plus server-side
// hashing of a plaintext password when AllowPlaintextPassword (dev) is set.
func (s *AuthService) verifyPassword(password, hashedPassword string) bool {
	ok, _ := s.matchPassword(password, password, "", hashedPassword)
	return ok
}

// HashPassword creates a SHA-256 hash of a password.
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}
