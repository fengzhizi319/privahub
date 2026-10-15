// Copyright 2026 Privahub Authors. Licensed under the Apache License, Version 2.0.
package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestRBAC_RolesAreDatabaseLocalAndRevocationIsImmediate(t *testing.T) {
	ctx := context.Background()
	first, second := setupRBACTestDB(t), setupRBACTestDB(t)
	user := &auth.UserContext{Name: "same-name", OwnerType: auth.OwnerTypeCenter}
	grant := model.SysUserPermissionRelDO{UserKey: user.Name, UserType: "USER", TargetType: "ROLE", TargetCode: auth.RoleAdmin}
	require.NoError(t, first.Create(&grant).Error)
	require.True(t, IsAdmin(ctx, first, user))
	require.False(t, IsAdmin(ctx, second, user), "same username in another database must not inherit a cached role")
	require.NoError(t, first.Delete(&grant).Error)
	require.False(t, IsAdmin(ctx, first, user), "revocation must not wait for a process-local TTL")
	require.NoError(t, first.Create(&model.SysUserPermissionRelDO{UserKey: user.Name, UserType: "NODE", TargetType: "ROLE", TargetCode: auth.RoleAdmin}).Error)
	require.False(t, IsAdmin(ctx, first, user), "a same-named virtual node cannot grant a human account ADMIN")
	require.NoError(t, first.Create(&model.SysUserPermissionRelDO{UserKey: user.Name, UserType: "USER", TargetType: "ROLE", TargetCode: auth.RoleAuditor}).Error)
	require.True(t, IsAuditor(ctx, first, user))
	require.NoError(t, first.Migrator().DropTable(&model.SysUserPermissionRelDO{}))
	require.False(t, IsAuditor(ctx, first, user), "storage failure must not reuse a previously granted role")
	require.False(t, CheckInterfaceResource(ctx, nil, fakePlatform{edge: true}, nil, "PRJ_CREATE"))
}

func TestSessionAuth_AmbiguousCredentialsNeverSelectAPrivilegedFallback(t *testing.T) {
	sessions := &fakeSessions{valid: map[string]*auth.UserContext{"low": {Name: "low"}, "high": {Name: "high"}}}
	for _, tc := range []struct {
		name    string
		headers map[string][]string
		status  int
	}{
		{"legacy", map[string][]string{"User-Token": {"low"}}, http.StatusOK},
		{"bearer whitespace", map[string][]string{"Authorization": {"Bearer\tlow"}}, http.StatusOK},
		{"same credential", map[string][]string{"User-Token": {"low"}, "Authorization": {"Bearer low"}}, http.StatusOK},
		{"different identities", map[string][]string{"User-Token": {"low"}, "Authorization": {"Bearer high"}}, http.StatusUnauthorized},
		{"malformed bearer", map[string][]string{"Authorization": {"Bearer"}}, http.StatusUnauthorized},
		{"other scheme", map[string][]string{"Authorization": {"Basic high"}}, http.StatusUnauthorized},
		{"duplicate bearer", map[string][]string{"Authorization": {"Bearer low", "Bearer high"}}, http.StatusUnauthorized},
		{"blank header", map[string][]string{"User-Token": {""}}, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := sessionRouter(nil, sessions, true)
			req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
			// Even a valid ambient cookie must never rescue a malformed explicit credential.
			req.AddCookie(&http.Cookie{Name: "privahub_session", Value: "high"})
			for key, values := range tc.headers {
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			require.Equal(t, tc.status, out.Code)
			if tc.status == http.StatusOK {
				require.Contains(t, out.Body.String(), `"low"`, "the parser must choose the identity it validated")
			}
		})
	}
}
