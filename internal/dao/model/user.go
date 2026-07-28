package model

import "time"

// UserAccountsDO represents a user account entity.
type UserAccountsDO struct {
	BaseDO
	Name                      string     `gorm:"type:varchar(128);not null" json:"name"`
	PasswordHash              string     `gorm:"type:varchar(128);not null" json:"-"`
	OwnerType                 string     `gorm:"type:varchar(16);not null;default:'CENTER'" json:"owner_type"`
	OwnerID                   string     `gorm:"type:varchar(64);not null;default:'kuscia-system'" json:"owner_id"`
	PasswdResetFailedAttempts *int       `json:"passwd_reset_failed_attempts"`
	GmtPasswdResetRelease     *time.Time `json:"gmt_passwd_reset_release"`
	FailedAttempts            *int       `json:"failed_attempts"`
	LockedInvalidTime         *time.Time `json:"locked_invalid_time"`
}

func (UserAccountsDO) TableName() string { return "user_accounts" }

// UserTokensDO represents a user session token.
type UserTokensDO struct {
	BaseDO
	Name        string     `gorm:"type:varchar(128);not null" json:"name"`
	Token       string     `gorm:"type:varchar(64)" json:"token"`
	GmtToken    *time.Time `json:"gmt_token"`
	SessionData string     `gorm:"type:text" json:"session_data"`
}

func (UserTokensDO) TableName() string { return "user_tokens" }

// SysResourceDO represents a system API resource for RBAC.
type SysResourceDO struct {
	BaseDO
	ResourceType string `gorm:"type:varchar(16);not null;index:key_resource_type" json:"resource_type"` // API / NODE
	ResourceCode string `gorm:"uniqueIndex;type:varchar(64);not null" json:"resource_code"`
	ResourceName string `gorm:"type:varchar(64)" json:"resource_name"`
}

func (SysResourceDO) TableName() string { return "sys_resource" }

// SysRoleDO represents a system role.
type SysRoleDO struct {
	BaseDO
	RoleCode string `gorm:"uniqueIndex;type:varchar(64);not null" json:"role_code"`
	RoleName string `gorm:"type:varchar(64)" json:"role_name"`
}

func (SysRoleDO) TableName() string { return "sys_role" }

// SysRoleResourceRelDO maps roles to resources.
type SysRoleResourceRelDO struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	RoleCode     string    `gorm:"uniqueIndex:uniq_role_code_resource_code;type:varchar(64);not null" json:"role_code"`
	ResourceCode string    `gorm:"uniqueIndex:uniq_role_code_resource_code;type:varchar(64);not null" json:"resource_code"`
	GmtCreate    time.Time `gorm:"autoCreateTime" json:"gmt_create"`
	GmtModified  time.Time `gorm:"autoUpdateTime" json:"gmt_modified"`
}

func (SysRoleResourceRelDO) TableName() string { return "sys_role_resource_rel" }

// SysUserPermissionRelDO maps users to roles.
type SysUserPermissionRelDO struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserType    string    `gorm:"type:varchar(16);not null" json:"user_type"`
	UserKey     string    `gorm:"uniqueIndex:uniq_user_key_target_code;type:varchar(64);not null" json:"user_key"`
	TargetType  string    `gorm:"type:varchar(16);not null;default:'ROLE'" json:"target_type"`
	TargetCode  string    `gorm:"uniqueIndex:uniq_user_key_target_code;type:varchar(16);not null" json:"target_code"`
	GmtCreate   time.Time `gorm:"autoCreateTime" json:"gmt_create"`
	GmtModified time.Time `gorm:"autoUpdateTime" json:"gmt_modified"`
}

func (SysUserPermissionRelDO) TableName() string { return "sys_user_permission_rel" }

// SysUserNodeRelDO maps users to accessible nodes.
type SysUserNodeRelDO struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      string    `gorm:"uniqueIndex:uniq_user_id_node_id;type:varchar(64);not null" json:"user_id"`
	NodeID      string    `gorm:"uniqueIndex:uniq_user_id_node_id;type:varchar(64);not null" json:"node_id"`
	GmtCreate   time.Time `gorm:"autoCreateTime" json:"gmt_create"`
	GmtModified time.Time `gorm:"autoUpdateTime" json:"gmt_modified"`
}

func (SysUserNodeRelDO) TableName() string { return "sys_user_node_rel" }
