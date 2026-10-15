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
	"time"

	"github.com/fengzhizi319/privahub/internal/audit"
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
	// ErrPasswordTooWeak: the new password hash is in the weak dictionary.
	ErrPasswordTooWeak = errors.New("password too weak")
	// ErrPasswordReused: the new password equals a recently used one.
	ErrPasswordReused = errors.New("password recently used")
	// ErrRefreshTokenReused: a rotated (already used) refresh token was
	// presented again; the whole session has been revoked.
	ErrRefreshTokenReused = errors.New("refresh token reuse detected")
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

// errLoginFailed is the single, uniform login failure: unknown user, wrong
// password and "locked but wrong password" are indistinguishable, and the
// remaining attempts are never disclosed.
func errLoginFailed() *AuthError {
	return newAuthError(ErrInvalidPassword, errcode.AuthLoginFailed)
}

// AuthOptions configures AuthService behaviour (see config.AuthConfig).
type AuthOptions struct {
	// MaxAttempts is the number of failed logins from one client IP before
	// that (account, IP) pair is locked (default 5).
	MaxAttempts int
	// AccountMaxAttempts is the number of failed logins across all IPs before
	// the account itself is locked (default 4 x MaxAttempts). It is higher
	// than MaxAttempts so a single attacker cannot lock an admin out.
	AccountMaxAttempts int
	// LockDuration is the lock period (default 30m).
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
	if o.AccountMaxAttempts <= 0 {
		o.AccountMaxAttempts = 4 * o.MaxAttempts
	}
	if o.AccountMaxAttempts < o.MaxAttempts {
		o.AccountMaxAttempts = o.MaxAttempts
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
//	  passwordHash    = lowercase hex SHA-256(password)   — the standard scheme.
//	  passwordHashSm3 = lowercase hex SM3(password)       — optional; only used for
//	                    accounts migrated from Java SecretPad (stored SM3 hashes).
//	  password        = accepted as an alias of passwordHash (the v1alpha1 route of
//	                    the new frontend sends the SHA-256 hex in this field). Only in
//	                    dev/test profiles is a raw plaintext password also hashed.
//
// Storage: user_accounts.password_hash holds sm3kdf:v1 = SM3-KDF(salt, passwordHash)
// (pkg/auth/password.go); legacy raw / wrapped rows are upgraded on login.
//
// Lockout: failures are counted per (account, client IP) — MaxAttempts locks
// the pair — and per account — AccountMaxAttempts (higher) locks the account
// for every IP. Unknown accounts run a dummy KDF and are counted the same way,
// every failure returns the same AuthLoginFailed error, and "locked" is only
// revealed to a caller that presented the correct password.
//
// Sessions: user_tokens stores the SM3 digest of the access token, the digest
// of the only valid refresh token JTI (one-time-use rotation with reuse
// detection) and the account password version; a password change bumps the
// version and clears every session.
type AuthService struct {
	userRepo   repository.UserAccountsRepository
	tokenRepo  repository.UserTokensRepository
	jwtManager *auth.JWTManager
	db         *gorm.DB
	env        *EnvService
	opts       AuthOptions

	// throttle holds the per-(account, IP) and unknown-account counters.
	throttle *loginThrottle

	now func() time.Time
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
		throttle:   newLoginThrottle(),
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

	// ClientIP is set by the handler (gin ClientIP, trusted proxies aware).
	ClientIP string `json:"-"`
}

// LoginResponse is the Java UserContextDTO plus the Go token pair fields
// (access_token/refresh_token/expires_at/token_type) and a legacy `user`
// object kept for older frontend builds. mustChangePassword (UserContext)
// tells the frontend to open the change-password screen.
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

// loginCandidates derives the password candidates of a login request.
func (s *AuthService) loginCandidates(req *LoginRequest) auth.PasswordCandidates {
	c := auth.PasswordCandidates{SM3: strings.TrimSpace(req.PasswordHashSm3)}
	switch {
	case hexHash.MatchString(strings.TrimSpace(req.PasswordHash)):
		c.SHA256 = strings.TrimSpace(req.PasswordHash)
	case hexHash.MatchString(strings.TrimSpace(req.Password)):
		c.SHA256 = strings.TrimSpace(req.Password)
	case s.opts.AllowPlaintextPassword && req.Password != "":
		c.SHA256 = HashPassword(req.Password)
	case s.opts.AllowPlaintextPassword && req.PasswordHash != "":
		c.SHA256 = HashPassword(req.PasswordHash)
	}
	if c.SM3 != "" && !hexHash.MatchString(c.SM3) {
		c.SM3 = ""
	}
	return c
}

// Login authenticates a user and returns the session UserContext + tokens.
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	name := strings.TrimSpace(req.Username)
	ip := strings.TrimSpace(req.ClientIP)
	if name == "" || (req.PasswordHash == "" && req.Password == "" && req.PasswordHashSm3 == "") {
		return nil, newAuthError(ErrPasswordRequired, errcode.SystemValidationError, "name/passwordHash required")
	}
	cand := s.loginCandidates(req)
	now := s.now()
	pairKey := throttleKey(name, ip)

	user, err := s.userRepo.FindByName(ctx, name)
	if err != nil || user == nil {
		// Equalise timing with a real verification, count like a real account.
		auth.DummyPasswordWork()
		pairLocked := s.throttle.fail(pairKey, now, s.opts.MaxAttempts, s.opts.LockDuration)
		acctLocked := s.throttle.fail(accountKey(name), now, s.opts.AccountMaxAttempts, s.opts.LockDuration)
		recordAuthEvent(ctx, audit.ActionLogin, name, ip, audit.ResultFailure, "unknown_user")
		if pairLocked || acctLocked {
			recordAuthEvent(ctx, audit.ActionLockout, name, ip, audit.ResultFailure, "unknown_user_threshold")
		}
		return nil, errLoginFailed()
	}

	// Current lock state: (account, IP) pair or the whole account.
	var lockedUntil time.Time
	if until, ok := s.throttle.lockedUntil(pairKey, now); ok {
		lockedUntil = until
	}
	if user.LockedInvalidTime != nil {
		if user.LockedInvalidTime.After(now) {
			if user.LockedInvalidTime.After(lockedUntil) {
				lockedUntil = *user.LockedInvalidTime
			}
		} else {
			// Expired account lock: reset the account counter.
			s.updateUserFields(ctx, user, map[string]interface{}{"failed_attempts": nil, "locked_invalid_time": nil})
			user.FailedAttempts, user.LockedInvalidTime = nil, nil
		}
	}

	res := auth.VerifyStoredPassword(user.PasswordHash, cand)

	if !lockedUntil.IsZero() {
		// Locked: only a caller proving the password learns about the lock
		// (so they know to wait); everybody else gets the uniform error and
		// the counters are not extended.
		if res.OK {
			recordAuthEvent(ctx, audit.ActionLogin, name, ip, audit.ResultDenied, "locked")
			return nil, newAuthError(ErrUserLocked, errcode.AuthUserIsLocked, minutesLeft(lockedUntil, now))
		}
		recordAuthEvent(ctx, audit.ActionLogin, name, ip, audit.ResultFailure, "invalid_credentials_while_locked")
		return nil, errLoginFailed()
	}

	if !res.OK {
		s.failKnownUser(ctx, user, pairKey, ip, now)
		return nil, errLoginFailed()
	}

	// Success: reset counters, upgrade the stored hash when needed.
	s.throttle.reset(pairKey)
	if user.FailedAttempts != nil || user.LockedInvalidTime != nil {
		s.updateUserFields(ctx, user, map[string]interface{}{"failed_attempts": nil, "locked_invalid_time": nil})
		user.FailedAttempts, user.LockedInvalidTime = nil, nil
	}
	if res.NeedsRehash && cand.SHA256 != "" {
		if stored, err := auth.HashClientPassword(cand.SHA256); err == nil {
			if err := s.userRepo.UpdatePassword(ctx, user.Name, stored); err == nil {
				user.PasswordHash = stored
			}
		}
	}

	resp, err := s.createSession(ctx, user)
	if err != nil {
		return nil, err
	}
	reason := ""
	if user.MustChangePassword {
		reason = "must_change_password"
	}
	recordAuthEvent(ctx, audit.ActionLogin, name, ip, audit.ResultSuccess, reason)
	return resp, nil
}

// createSession issues the token pair and stores the session row.
func (s *AuthService) createSession(ctx context.Context, user *model.UserAccountsDO) (*LoginResponse, error) {
	uc := s.BuildUserContext(ctx, user)
	sid := uuid.New().String()
	uc.SessionID = sid
	uc.MustChangePassword = user.MustChangePassword
	uc.PwdVer = user.PasswordVersion
	uc.AuthorizationVersion = user.AuthorizationVersion
	tokenPair, err := s.jwtManager.GenerateSessionTokens(auth.SessionTokenParams{
		UserID: strconv.FormatInt(user.ID, 10), Username: user.Name, OwnerType: user.OwnerType,
		OwnerID: user.OwnerID, SessionID: sid, PwdVer: user.PasswordVersion,
	})
	if err != nil {
		return nil, err
	}
	if err := s.storeSession(ctx, uc, tokenPair); err != nil {
		return nil, err
	}
	uc.Token = tokenPair.AccessToken
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

// failKnownUser counts a failed login of an existing account: the (account,
// IP) pair locks after MaxAttempts, the account after AccountMaxAttempts.
func (s *AuthService) failKnownUser(ctx context.Context, user *model.UserAccountsDO, pairKey, ip string, now time.Time) {
	recordAuthEvent(ctx, audit.ActionLogin, user.Name, ip, audit.ResultFailure, "invalid_credentials")
	if s.throttle.fail(pairKey, now, s.opts.MaxAttempts, s.opts.LockDuration) {
		recordAuthEvent(ctx, audit.ActionLockout, user.Name, ip, audit.ResultFailure, "account_ip_threshold")
	}
	attempts := 1
	if user.FailedAttempts != nil {
		attempts = *user.FailedAttempts + 1
	}
	user.FailedAttempts = &attempts
	fields := map[string]interface{}{"failed_attempts": attempts}
	if attempts >= s.opts.AccountMaxAttempts {
		lock := now.Add(s.opts.LockDuration)
		user.LockedInvalidTime = &lock
		fields["locked_invalid_time"] = lock
		recordAuthEvent(ctx, audit.ActionLockout, user.Name, ip, audit.ResultFailure, "account_threshold")
	}
	s.updateUserFields(ctx, user, fields)
}

// updateUserFields writes selected columns (targeted update so concurrent
// password changes are never overwritten); without a DB handle it falls back
// to the repository full-row update.
func (s *AuthService) updateUserFields(ctx context.Context, user *model.UserAccountsDO, fields map[string]interface{}) {
	if s.db != nil {
		_ = s.db.WithContext(ctx).Model(&model.UserAccountsDO{}).Where("id = ?", user.ID).Updates(fields).Error
		return
	}
	_ = s.userRepo.Update(ctx, user)
}

func constEq(a, b string) bool { return hmac.Equal([]byte(a), []byte(b)) }

func minutesLeft(until, now time.Time) string {
	m := int(until.Sub(now).Minutes() + 0.999)
	if m < 1 {
		m = 1
	}
	return strconv.Itoa(m)
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

// HashPassword creates a SHA-256 hash of a password.
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}
