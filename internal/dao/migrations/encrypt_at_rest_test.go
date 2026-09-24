package migration

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	pcrypto "github.com/fengzhizi319/privahub/pkg/crypto"
)

func TestSecretsEncryptedAtRest(t *testing.T) {
	db := setupMigrateTestDB(t)
	ctx := context.Background()

	// Legacy plaintext rows written before encryption was enabled.
	pcrypto.SetDefault(nil)
	require.NoError(t, db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice", Token: "plain-node-token", InstToken: "plain-inst-jwt"}).Error)
	require.NoError(t, db.Create(&model.DatasourceDO{DatasourceID: "ds1", Name: "ds", OwnerID: "alice", ConnectionInfo: `{"accessKeySecret":"sk"}`}).Error)

	kr1, err := pcrypto.NewKeyring(map[string]string{"v1": strings.Repeat("11", 32)}, "v1")
	require.NoError(t, err)
	pcrypto.SetDefault(kr1)
	defer pcrypto.SetDefault(nil)

	require.NoError(t, EncryptSecretsAtRest(ctx, db, zap.NewNop()))
	raw := func(q string) string {
		var s string
		require.NoError(t, db.Raw(q).Row().Scan(&s))
		return s
	}
	assert.True(t, strings.HasPrefix(raw("SELECT token FROM node WHERE node_id='alice'"), "enc:v1:"))
	assert.True(t, strings.HasPrefix(raw("SELECT inst_token FROM node WHERE node_id='alice'"), "enc:v1:"))
	assert.True(t, strings.HasPrefix(raw("SELECT connection_info FROM datasource WHERE datasource_id='ds1'"), "enc:v1:"))

	// Reads decrypt transparently.
	var n model.NodeDO
	require.NoError(t, db.Where("node_id = ?", "alice").First(&n).Error)
	assert.Equal(t, "plain-node-token", n.Token)
	assert.Equal(t, "plain-inst-jwt", n.InstToken)

	// Idempotent.
	before := raw("SELECT token FROM node WHERE node_id='alice'")
	require.NoError(t, EncryptSecretsAtRest(ctx, db, zap.NewNop()))
	assert.Equal(t, before, raw("SELECT token FROM node WHERE node_id='alice'"))

	// Struct writes and map writes are sealed.
	n.Token = "new-token"
	require.NoError(t, db.Save(&n).Error)
	assert.Equal(t, "new-token", n.Token, "caller struct keeps plaintext")
	assert.True(t, strings.HasPrefix(raw("SELECT token FROM node WHERE node_id='alice'"), "enc:v1:"))
	require.NoError(t, db.Model(&model.NodeDO{}).Where("node_id = ?", "alice").
		Updates(map[string]interface{}{"inst_token": "map-jwt", "inst_token_state": "UNUSED"}).Error)
	assert.True(t, strings.HasPrefix(raw("SELECT inst_token FROM node WHERE node_id='alice'"), "enc:v1:"))
	require.NoError(t, db.Model(&model.NodeDO{}).Where("node_id = ?", "alice").Update("token", "upd-token").Error)
	assert.True(t, strings.HasPrefix(raw("SELECT token FROM node WHERE node_id='alice'"), "enc:v1:"))
	var n2 model.NodeDO
	require.NoError(t, db.Where("node_id = ?", "alice").First(&n2).Error)
	assert.Equal(t, "upd-token", n2.Token)
	assert.Equal(t, "map-jwt", n2.InstToken)

	// Key rotation: v2 active, v1 kept for decryption; migration re-seals.
	kr2, err := pcrypto.NewKeyring(map[string]string{"v1": strings.Repeat("11", 32), "v2": strings.Repeat("22", 32)}, "v2")
	require.NoError(t, err)
	pcrypto.SetDefault(kr2)
	require.NoError(t, EncryptSecretsAtRest(ctx, db, zap.NewNop()))
	assert.True(t, strings.HasPrefix(raw("SELECT token FROM node WHERE node_id='alice'"), "enc:v2:"))
	var n3 model.NodeDO
	require.NoError(t, db.Where("node_id = ?", "alice").First(&n3).Error)
	assert.Equal(t, "upd-token", n3.Token)
}
