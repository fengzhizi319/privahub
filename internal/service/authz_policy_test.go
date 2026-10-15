// Copyright 2026 Privahub Authors. Licensed under the Apache License, Version 2.0.
package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestUserPolicyChangeRevokesAccessAndRefresh(t *testing.T) {
	for _, change := range []struct {
		name string
		req  UpdateUserRequest
	}{
		{"roles", UpdateUserRequest{RoleCodes: []string{}}},
		{"nodes", UpdateUserRequest{NodeIDs: []string{}}},
		{"owner", UpdateUserRequest{OwnerType: "EDGE", OwnerID: "alice"}},
	} {
		t.Run(change.name, func(t *testing.T) {
			authSvc, db := newTestAuthService(t)
			users, _ := newPwdTestUserServiceDB(db)
			createUser(t, db, "subject", "Str0ng-Pass#1", "CENTER", "kuscia-system")
			first := loginOK(t, authSvc, "subject", "Str0ng-Pass#1")
			second := loginOK(t, authSvc, "subject", "Str0ng-Pass#1")
			// Capture a row read by an in-flight login before the permission change.
			var stale model.UserAccountsDO
			require.NoError(t, db.Where("name = ?", "subject").First(&stale).Error)
			change.req.Name = "subject"
			require.NoError(t, users.UpdateUser(context.Background(), &change.req))
			for _, session := range []*LoginResponse{first, second} {
				_, err := authSvc.ValidateSession(context.Background(), session.Token)
				require.Error(t, err)
				_, err = authSvc.RefreshToken(context.Background(), session.RefreshToken)
				require.Error(t, err)
			}
			// Simulate stale login completing after the revocation transaction: the
			// grant epoch must reject it, even though it inserted a new session row.
			late, err := authSvc.createSession(context.Background(), &stale)
			require.NoError(t, err)
			_, err = authSvc.ValidateSession(context.Background(), late.Token)
			require.Error(t, err, "late stale login must not resurrect old privileges")
			_, err = authSvc.RefreshToken(context.Background(), late.RefreshToken)
			require.Error(t, err)
			fresh := loginOK(t, authSvc, "subject", "Str0ng-Pass#1")
			_, err = authSvc.ValidateSession(context.Background(), fresh.Token)
			require.NoError(t, err, "fresh login uses the new policy epoch")
		})
	}
}

func TestUserPolicyChangeRollbackRetainsOwnerRolesAndSessions(t *testing.T) {
	authSvc, db := newTestAuthService(t)
	users, _ := newPwdTestUserServiceDB(db)
	createUser(t, db, "subject", "Str0ng-Pass#1", "CENTER", "kuscia-system")
	require.NoError(t, db.Create(&model.SysUserPermissionRelDO{UserType: "USER", UserKey: "subject", TargetType: "ROLE", TargetCode: "ADMIN"}).Error)
	session := loginOK(t, authSvc, "subject", "Str0ng-Pass#1")
	// A real database failure after the account update exercises transaction
	// rollback, rather than merely asserting the happy-path SQL sequence.
	require.NoError(t, db.Exec(`CREATE TRIGGER deny_test_role BEFORE INSERT ON sys_user_permission_rel WHEN NEW.target_code = 'FAIL' BEGIN SELECT RAISE(ABORT, 'injected role failure'); END`).Error)
	err := users.UpdateUser(context.Background(), &UpdateUserRequest{Name: "subject", OwnerType: "EDGE", OwnerID: "alice", RoleCodes: []string{"FAIL"}})
	require.Error(t, err)
	var account model.UserAccountsDO
	require.NoError(t, db.Where("name = ?", "subject").First(&account).Error)
	require.Equal(t, "CENTER", account.OwnerType)
	require.Equal(t, "kuscia-system", account.OwnerID)
	require.Zero(t, account.AuthorizationVersion)
	var roles []model.SysUserPermissionRelDO
	require.NoError(t, db.Where("user_key = ?", "subject").Find(&roles).Error)
	require.Len(t, roles, 1)
	require.Equal(t, "ADMIN", roles[0].TargetCode)
	_, err = authSvc.ValidateSession(context.Background(), session.Token)
	require.NoError(t, err, "failed policy change must not revoke the original session")
}

func TestArchivedProjectDoesNotWidenTenantScope(t *testing.T) {
	_, db := newTestAuthService(t)
	require.NoError(t, db.AutoMigrate(&model.ProjectDO{}, &model.ProjectInstDO{}))
	require.NoError(t, db.Create(&model.ProjectDO{ProjectID: "archived", Name: "private", Status: 2}).Error)
	require.NoError(t, db.Create(&model.ProjectNodeDO{ProjectID: "archived", NodeID: "alice"}).Error)
	check := NewDataResourceAuth(db, NewEnvService("CENTER", "kuscia-system", "", nil))
	require.True(t, check.Check(context.Background(), &auth.UserContext{OwnerType: "EDGE", OwnerID: "alice"}, DataResourceProjectID, "archived"))
	require.False(t, check.Check(context.Background(), &auth.UserContext{OwnerType: "EDGE", OwnerID: "bob"}, DataResourceProjectID, "archived"))
}

func TestHumanRoleUpdatesDoNotOverwriteSameNamedNodeGrants(t *testing.T) {
	authSvc, db := newTestAuthService(t)
	users, _ := newPwdTestUserServiceDB(db)
	_, err := users.CreateUser(context.Background(), &CreateUserRequest{Name: "edge-account", Password: "Str0ng-Pass#1", OwnerType: "EDGE", OwnerID: "alice", RoleCodes: []string{"READ"}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.SysResourceDO{ResourceType: "API", ResourceCode: "PRJ_CREATE"}).Error)
	require.NoError(t, db.Create(&model.SysRoleResourceRelDO{RoleCode: "READ", ResourceCode: "PRJ_CREATE"}).Error)
	require.Equal(t, []string{"PRJ_CREATE"}, QueryAPIResourceCodes(context.Background(), db, auth.PermissionUserTypeEdgeUser, "edge-account"))
	require.NoError(t, db.Create(&model.SysUserPermissionRelDO{UserType: "NODE", UserKey: "edge-account", TargetType: "ROLE", TargetCode: "EDGE_NODE"}).Error)
	require.NoError(t, users.UpdateUser(context.Background(), &UpdateUserRequest{Name: "edge-account", RoleCodes: []string{"READ", "READ"}}))
	var grants []model.SysUserPermissionRelDO
	require.NoError(t, db.Where("user_key = ?", "edge-account").Find(&grants).Error)
	require.Len(t, grants, 2, "deduplicate human grants while preserving NODE grants")
	fresh := loginOK(t, authSvc, "edge-account", "Str0ng-Pass#1")
	require.Equal(t, []string{"PRJ_CREATE"}, fresh.APIResources)
	require.NoError(t, users.DeleteUser(context.Background(), "edge-account"))
	grants = nil
	require.NoError(t, db.Where("user_key = ?", "edge-account").Find(&grants).Error)
	require.Len(t, grants, 1)
	require.Equal(t, "NODE", grants[0].UserType)
	_, err = authSvc.ValidateSession(context.Background(), fresh.Token)
	require.Error(t, err)
}

func TestAuthorizationEpochUpgradePreservesExistingSessions(t *testing.T) {
	svc, db := newTestAuthService(t)
	createUser(t, db, "existing", "Str0ng-Pass#1", "CENTER", "kuscia-system")
	session := loginOK(t, svc, "existing", "Str0ng-Pass#1")
	// Recreate the pre-upgrade schema while keeping its account/session data.
	require.NoError(t, db.Migrator().DropColumn(&model.UserAccountsDO{}, "AuthorizationVersion"))
	require.NoError(t, db.Migrator().DropColumn(&model.UserTokensDO{}, "AuthorizationVersion"))
	require.NoError(t, db.AutoMigrate(&model.UserAccountsDO{}, &model.UserTokensDO{}))
	_, err := svc.ValidateSession(context.Background(), session.Token)
	require.NoError(t, err, "zero-default grant epochs keep pre-upgrade sessions usable")
	_, err = svc.RefreshToken(context.Background(), session.RefreshToken)
	require.NoError(t, err)
}
