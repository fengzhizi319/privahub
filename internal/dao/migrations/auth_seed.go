package migration

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SeedOptions controls environment-dependent seed data (initial admin,
// platform owner type, node permissions).
type SeedOptions struct {
	// Dev enables the development default password (12345678) when
	// AdminPassword is empty.
	Dev bool
	// AdminName is the initial admin account name (default "admin").
	AdminName string
	// AdminPassword is the initial admin plaintext password; when empty and
	// not Dev, a random password is generated and written once to
	// InitialPasswordFile with mode 0600 (never logged).
	AdminPassword       string
	InitialPasswordFile string
	// PlatformType is the normalized platform (CENTER/EDGE/AUTONOMY/TEST);
	// it determines the admin owner type like Java DbDataInit.
	PlatformType string
	// NodeID is the platform node id (admin owner id).
	NodeID string
}

// DevSeedOptions returns the development defaults (admin/12345678, CENTER).
func DevSeedOptions() SeedOptions {
	return SeedOptions{Dev: true, AdminName: "admin", PlatformType: "CENTER", NodeID: "kuscia-system"}
}

func (o *SeedOptions) normalize() {
	if o.AdminName == "" {
		o.AdminName = "admin"
	}
	if o.PlatformType == "" {
		o.PlatformType = "CENTER"
	}
	if o.NodeID == "" {
		o.NodeID = "kuscia-system"
	}
	if o.InitialPasswordFile == "" {
		o.InitialPasswordFile = "./config/initial-admin-password"
	}
}

// ownerTypeForPlatform mirrors Java UserOwnerTypeEnum.fromString(platformType).
func ownerTypeForPlatform(platform string) string {
	switch strings.ToUpper(platform) {
	case "EDGE", "TEST":
		return "EDGE"
	case "AUTONOMY", "P2P":
		return "P2P"
	default:
		return "CENTER"
	}
}

// seedAdminUser creates the initial admin if it does not exist. An existing
// account is never modified (Java DbDataInit P0-1 rule).
func seedAdminUser(ctx context.Context, db *gorm.DB, log *zap.Logger, opts SeedOptions) error {
	opts.normalize()
	var count int64
	if err := db.WithContext(ctx).Model(&model.UserAccountsDO{}).Where("name = ?", opts.AdminName).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	password := opts.AdminPassword
	generated := false
	if password == "" {
		if opts.Dev {
			password = "12345678"
		} else {
			p, err := randomPassword(16)
			if err != nil {
				return err
			}
			password, generated = p, true
		}
	}

	ownerID := opts.NodeID
	if ownerTypeForPlatform(opts.PlatformType) == "CENTER" && ownerID == "" {
		ownerID = "kuscia-system"
	}
	admin := &model.UserAccountsDO{
		Name:         opts.AdminName,
		PasswordHash: hashPassword(password),
		OwnerType:    ownerTypeForPlatform(opts.PlatformType),
		OwnerID:      ownerID,
	}
	if generated {
		// Write the password before creating the account so a failure does
		// not leave an account with an unknown password.
		if err := writeInitialPassword(opts.InitialPasswordFile, password); err != nil {
			return fmt.Errorf("cannot write initial admin password to %s (set auth.admin_password / PRIVAHUB_AUTH_ADMIN_PASSWORD instead): %w",
				opts.InitialPasswordFile, err)
		}
	}
	if err := db.WithContext(ctx).Create(admin).Error; err != nil {
		return err
	}
	if log != nil {
		switch {
		case generated:
			abs, _ := filepath.Abs(opts.InitialPasswordFile)
			log.Warn("initial admin password generated; read it once, log in, change it, then delete the file",
				zap.String("user", opts.AdminName), zap.String("file", abs))
		case opts.AdminPassword != "":
			log.Info("initial admin created with the configured password", zap.String("user", opts.AdminName))
		default:
			log.Warn("initial admin created with the DEV default password", zap.String("user", opts.AdminName))
		}
	}
	return nil
}

func writeInitialPassword(path, password string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#%^&*"

func randomPassword(n int) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := range b {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = passwordAlphabet[v.Int64()]
	}
	return string(b), nil
}

// javaRoles mirrors the Java sys_role seeds (+ Go ADMIN / CENTER_ADMIN).
var javaRoles = map[string]string{
	"ADMIN":        "管理员",
	"CENTER_ADMIN": "Center admin",
	"EDGE_NODE":    "Edge node rpc",
	"EDGE_USER":    "Edge 用户",
	"P2P_NODE":     "P2P 用户",
}

// seedRBAC seeds sys_role / sys_resource / sys_role_resource_rel from the Java
// SQL seeds, grants ADMIN to the initial admin, and grants the node RPC role
// (EDGE_NODE, or P2P_NODE on AUTONOMY) to every known node (Java NodeManager
// does this when a node is created). Idempotent.
func seedRBAC(ctx context.Context, db *gorm.DB, opts SeedOptions) error {
	opts.normalize()
	tx := db.WithContext(ctx)
	for code, name := range javaRoles {
		if err := tx.Where(model.SysRoleDO{RoleCode: code}).
			Attrs(model.SysRoleDO{RoleName: name}).FirstOrCreate(&model.SysRoleDO{}).Error; err != nil {
			return err
		}
	}
	for _, code := range javaAPIResources {
		if err := tx.Where(model.SysResourceDO{ResourceCode: code}).
			Attrs(model.SysResourceDO{ResourceType: "API", ResourceName: code}).
			FirstOrCreate(&model.SysResourceDO{}).Error; err != nil {
			return err
		}
	}
	roleResources := map[string][]string{"ADMIN": {"ALL_INTERFACE_RESOURCE"}}
	for k, v := range javaRoleResources {
		roleResources[k] = append(roleResources[k], v...)
	}
	for role, codes := range roleResources {
		for _, code := range codes {
			if err := tx.Where(model.SysRoleResourceRelDO{RoleCode: role, ResourceCode: code}).
				FirstOrCreate(&model.SysRoleResourceRelDO{}).Error; err != nil {
				return err
			}
		}
	}

	// Initial admin -> ADMIN role.
	if err := tx.Where(model.SysUserPermissionRelDO{UserKey: opts.AdminName, TargetCode: "ADMIN"}).
		Attrs(model.SysUserPermissionRelDO{UserType: "USER", TargetType: "ROLE"}).
		FirstOrCreate(&model.SysUserPermissionRelDO{}).Error; err != nil {
		return err
	}

	// Node RPC permissions for existing nodes.
	nodeRole := "EDGE_NODE"
	if strings.EqualFold(opts.PlatformType, "AUTONOMY") {
		nodeRole = "P2P_NODE"
	}
	var nodeIDs []string
	if err := tx.Model(&model.NodeDO{}).Pluck("node_id", &nodeIDs).Error; err != nil {
		return err
	}
	for _, id := range nodeIDs {
		if err := EnsureNodePermission(ctx, db, id, nodeRole); err != nil {
			return err
		}
	}
	return nil
}

// EnsureNodePermission grants a node-RPC role (EDGE_NODE / P2P_NODE) to a node
// (sys_user_permission_rel user_type=NODE), like Java NodeManager on node
// creation. Node creation code should call it for new nodes.
func EnsureNodePermission(ctx context.Context, db *gorm.DB, nodeID, role string) error {
	return db.WithContext(ctx).
		Where(model.SysUserPermissionRelDO{UserKey: nodeID, TargetCode: role}).
		Attrs(model.SysUserPermissionRelDO{UserType: "NODE", TargetType: "ROLE"}).
		FirstOrCreate(&model.SysUserPermissionRelDO{}).Error
}

// javaAPIResources are the Java sys_resource API codes (config/schema/*/V*.sql)
// plus codes referenced by Java @ApiResource annotations but never seeded.
var javaAPIResources = []string{
	"ALL_INTERFACE_RESOURCE",
	"AUTH_LOGIN",
	"AUTH_LOGOUT",
	"DATATABLE_CREATE",
	"DATATABLE_DELETE",
	"DATATABLE_GET",
	"DATATABLE_LIST",
	"DATA_CREATE",
	"DATA_CREATE_DATA",
	"DATA_DOWNLOAD",
	"DATA_LIST_DATASOURCE",
	"DATA_UPLOAD",
	"ENV_GET",
	"GRAPH_COMM_BATH",
	"GRAPH_COMM_GET",
	"GRAPH_COMM_I18N",
	"GRAPH_COMM_LIST",
	"GRAPH_CREATE",
	"GRAPH_DELETE",
	"GRAPH_DETAIL",
	"GRAPH_LIST",
	"GRAPH_META_UPDATE",
	"GRAPH_NODE_LOGS",
	"GRAPH_NODE_OUTPUT",
	"GRAPH_NODE_STATUS",
	"GRAPH_NODE_UPDATE",
	"GRAPH_START",
	"GRAPH_STOP",
	"GRAPH_UPDATE",
	"INDEX",
	"NODE_CREATE",
	"NODE_DELETE",
	"NODE_GET",
	"NODE_LIST",
	"NODE_NEW_TOKEN",
	"NODE_PAGE",
	"NODE_REFRESH",
	"NODE_RESULT_DETAIL",
	"NODE_RESULT_LIST",
	"NODE_ROUTE_CREATE",
	"NODE_ROUTE_DELETE",
	"NODE_ROUTE_GET",
	"NODE_ROUTE_LIST_NODE",
	"NODE_ROUTE_PAGE",
	"NODE_ROUTE_REFRESH",
	"NODE_ROUTE_UPDATE",
	"NODE_TOKEN",
	"NODE_UPDATE",
	"NODE_USER_CREATE",
	"NODE_USER_LIST_BY_NODE",
	"NODE_USER_NODE_LIST",
	"NODE_USER_OTHER_NODE_BASE_INFO_LIST",
	"NODE_USER_REMOTE_NODE_LIST",
	"NODE_USER_RESET_PWD",
	"PRJ_ADD_INST",
	"PRJ_ADD_NODE",
	"PRJ_ADD_TABLE",
	"PRJ_CREATE",
	"PRJ_DATATABLE_DELETE",
	"PRJ_DATATABLE_GET",
	"PRJ_DELETE",
	"PRJ_GET",
	"PRJ_JOB_GET",
	"PRJ_JOB_LIST",
	"PRJ_JOB_STOP",
	"PRJ_LIST",
	"PRJ_TASK_LOGS",
	"PRJ_TASK_OUTPUT",
	"PRJ_UPDATE",
	"REMOTE_USER_CREATE",
	"REMOTE_USER_LIST_BY_NODE",
	"REMOTE_USER_RESET_PWD",
	"SCHEDULED_CREATE",
	"SCHEDULED_ID",
	"SCHEDULED_OFFLINE",
	"SCHEDULED_PAGE",
	"USER_CREATE",
	"USER_GET",
	"USER_UPDATE_PWD",
	"PRJ_ARCHIVE",
	"PRJ_PARTICIPANTS",
	"INST_GET",
	"COMPONENT_VERSION_LIST",
}

// javaRoleResources are the Java sys_role_resource_rel seeds.
var javaRoleResources = map[string][]string{
	"CENTER_ADMIN": {
		"ALL_INTERFACE_RESOURCE",
	},
	"EDGE_NODE": {
		"DATATABLE_CREATE",
		"DATATABLE_DELETE",
		"DATA_CREATE",
		"DATA_CREATE_DATA",
		"ENV_GET",
		"INDEX",
		"NODE_ROUTE_DELETE",
		"NODE_ROUTE_UPDATE",
		"NODE_UPDATE",
		"NODE_USER_CREATE",
		"NODE_USER_LIST_BY_NODE",
		"NODE_USER_OTHER_NODE_BASE_INFO_LIST",
		"NODE_USER_RESET_PWD",
		"PRJ_ADD_TABLE",
		"PRJ_DATATABLE_DELETE",
		"PRJ_UPDATE",
	},
	"EDGE_USER": {
		"AUTH_LOGIN",
		"AUTH_LOGOUT",
		"ENV_GET",
		"GRAPH_COMM_BATH",
		"GRAPH_COMM_GET",
		"GRAPH_COMM_I18N",
		"GRAPH_COMM_LIST",
		"GRAPH_CREATE",
		"GRAPH_DELETE",
		"GRAPH_DETAIL",
		"GRAPH_LIST",
		"GRAPH_META_UPDATE",
		"GRAPH_NODE_LOGS",
		"GRAPH_NODE_OUTPUT",
		"GRAPH_NODE_STATUS",
		"GRAPH_NODE_UPDATE",
		"GRAPH_START",
		"GRAPH_STOP",
		"GRAPH_UPDATE",
		"INDEX",
		"NODE_USER_CREATE",
		"NODE_USER_LIST_BY_NODE",
		"NODE_USER_NODE_LIST",
		"NODE_USER_REMOTE_NODE_LIST",
		"NODE_USER_RESET_PWD",
		"PRJ_ADD_INST",
		"PRJ_ADD_NODE",
		"PRJ_ADD_TABLE",
		"PRJ_CREATE",
		"PRJ_DATATABLE_DELETE",
		"PRJ_DATATABLE_GET",
		"PRJ_DELETE",
		"PRJ_GET",
		"PRJ_JOB_GET",
		"PRJ_JOB_LIST",
		"PRJ_JOB_STOP",
		"PRJ_LIST",
		"PRJ_TASK_LOGS",
		"PRJ_TASK_OUTPUT",
		"PRJ_UPDATE",
		"SCHEDULED_CREATE",
		"SCHEDULED_ID",
		"SCHEDULED_OFFLINE",
		"SCHEDULED_PAGE",
		"USER_GET",
		"USER_UPDATE_PWD",
	},
}
