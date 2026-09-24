package auth

import (
	"context"
	"encoding/json"
)

// Owner types (Java UserOwnerTypeEnum).
const (
	OwnerTypeCenter = "CENTER"
	OwnerTypeEdge   = "EDGE"
	OwnerTypeP2P    = "P2P"
)

// Platform types (Java PlatformTypeEnum).
const (
	PlatformTypeCenter   = "CENTER"
	PlatformTypeEdge     = "EDGE"
	PlatformTypeAutonomy = "AUTONOMY"
	PlatformTypeTest     = "TEST"
)

// Permission user types (Java PermissionUserTypeEnum), used as
// sys_user_permission_rel.user_type.
const (
	PermissionUserTypeUser     = "USER"
	PermissionUserTypeEdgeUser = "EDGE_USER"
	PermissionUserTypeNode     = "NODE"
)

// Role codes (Java RoleCodeConstants + Go ADMIN).
const (
	RoleAdmin       = "ADMIN"
	RoleCenterAdmin = "CENTER_ADMIN"
	RoleEdgeNode    = "EDGE_NODE"
	RoleEdgeUser    = "EDGE_USER"
	RoleP2PNode     = "P2P_NODE"
	// RoleAuditor may read and verify the security audit trail (read-only).
	RoleAuditor = "AUDITOR"
)

// AllInterfaceResource grants access to every API resource code.
const AllInterfaceResource = "ALL_INTERFACE_RESOURCE"

// UserContext mirrors Java UserContextDTO: the per-request identity of the
// caller. For browser sessions it is the snapshot stored in
// user_tokens.session_data at login; for inner-port RPC it is a virtual user
// built from the `kuscia-origin-source` header.
//
// JSON field names follow the Java contract (camelCase).
type UserContext struct {
	Token          string   `json:"token"`
	Name           string   `json:"name"`
	PlatformType   string   `json:"platformType"`
	PlatformNodeID string   `json:"platformNodeId"`
	OwnerType      string   `json:"ownerType"`
	OwnerID        string   `json:"ownerId"`
	OwnerName      string   `json:"ownerName,omitempty"`
	ProjectIDs     []string `json:"projectIds"`
	APIResources   []string `json:"apiResources"`
	// VirtualUserForNode is only set (true) for inner-port node RPC callers.
	VirtualUserForNode *bool  `json:"virtualUserForNode,omitempty"`
	DeployMode         string `json:"deployMode"`

	// Go-only fields (not part of the Java DTO, omitted from JSON when empty).
	UserID    string `json:"userId,omitempty"`
	SessionID string `json:"-"` // stored in user_tokens.session_id
	// MustChangePassword restricts the session to user/get, user/updatePwd
	// and logout until the password is changed (generated initial admin
	// password, admin reset).
	MustChangePassword bool `json:"mustChangePassword,omitempty"`
	// PwdVer is the account password version the session was created with.
	PwdVer int64 `json:"-"`
}

// IsCenterOwner reports whether the user is a CENTER platform user (full data
// and interface permission in Java's aspects).
func (u *UserContext) IsCenterOwner() bool { return u != nil && u.OwnerType == OwnerTypeCenter }

// IsVirtualNode reports whether this is an inner-port node RPC identity.
func (u *UserContext) IsVirtualNode() bool {
	return u != nil && u.VirtualUserForNode != nil && *u.VirtualUserForNode
}

// ContainProjectID mirrors UserContextDTO.containProjectId.
func (u *UserContext) ContainProjectID(projectID string) bool {
	if u == nil || projectID == "" {
		return false
	}
	for _, p := range u.ProjectIDs {
		if p == projectID {
			return true
		}
	}
	return false
}

// ContainInterfaceResource mirrors UserContextDTO.containInterfaceResource.
func (u *UserContext) ContainInterfaceResource(code string) bool {
	if u == nil || code == "" {
		return false
	}
	for _, r := range u.APIResources {
		if r == code {
			return true
		}
	}
	return false
}

// Normalize replaces nil slices with empty ones so the JSON output never
// contains null arrays (the frontend zod schemas reject null).
func (u *UserContext) Normalize() *UserContext {
	if u.ProjectIDs == nil {
		u.ProjectIDs = []string{}
	}
	if u.APIResources == nil {
		u.APIResources = []string{}
	}
	return u
}

// ToJSON serializes the context (stored as user_tokens.session_data).
func (u *UserContext) ToJSON() string {
	b, _ := json.Marshal(u)
	return string(b)
}

// UserContextFromJSON parses a session_data snapshot.
func UserContextFromJSON(s string) (*UserContext, error) {
	var u UserContext
	if err := json.Unmarshal([]byte(s), &u); err != nil {
		return nil, err
	}
	return u.Normalize(), nil
}

// BoolPtr returns a pointer to b.
func BoolPtr(b bool) *bool { return &b }

const userContextKey contextKey = "user_context"

// ContextWithUser stores the UserContext in ctx (also sets the username key so
// UsernameFromContext keeps working).
func ContextWithUser(ctx context.Context, u *UserContext) context.Context {
	ctx = context.WithValue(ctx, userContextKey, u)
	ctx = context.WithValue(ctx, UsernameContextKey, u.Name)
	if u.UserID != "" {
		ctx = context.WithValue(ctx, UserIDContextKey, u.UserID)
	}
	return ctx
}

// UserFromContext returns the current request's UserContext (Java
// UserContext.getUser()).
func UserFromContext(ctx context.Context) (*UserContext, bool) {
	if ctx == nil {
		return nil, false
	}
	u, ok := ctx.Value(userContextKey).(*UserContext)
	return u, ok && u != nil
}
