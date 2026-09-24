package service

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

type recordingDbSyncSender struct {
	mu    sync.Mutex
	node  []string
	items []SyncData
}

func (r *recordingDbSyncSender) Send(_ context.Context, nodeID string, entities ...SyncData) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range entities {
		r.node = append(r.node, nodeID)
		r.items = append(r.items, e)
	}
	return nil
}

// On EDGE the TEE management records of push/pull/delete are pushed to the
// center through the DbSyncSender instead of being saved locally (Java
// saveTeeNodeDatatableManagementOrPush + DbSyncUtil.dbDataSyncToCenter).
func TestSaveTeeManagement_EdgePushesToCenter(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	seedTables(t, dt, fk)
	ctx := context.Background()
	dt.db.Create(&model.NodeRouteDO{RouteID: "r1", SrcNodeID: "alice", DstNodeID: "tee"})
	dt.db.Create(&model.NodeRouteDO{RouteID: "r2", SrcNodeID: "tee", DstNodeID: "alice"})

	var before int64
	dt.db.Model(&model.TeeNodeDatatableManagementDO{}).Count(&before)

	prevEnv := GlobalEnv()
	prevSender := CurrentDbSyncSender()
	rec := &recordingDbSyncSender{}
	SetGlobalEnv(NewEnvService("EDGE", "alice", "", nil))
	SetDbSyncSender(rec)
	t.Cleanup(func() {
		SetGlobalEnv(prevEnv)
		SetDbSyncSender(prevSender)
	})

	require.NoError(t, dt.PushDatatableToTee(ctx, &PushDatatableToTeeRequest{NodeID: "alice", DatatableID: "t2"}))
	_, err := NewTeePullService(dt).PullResult(ctx, &TeePullRequest{NodeID: "alice", ResourceID: "r-1", ProjectID: "p1"})
	require.NoError(t, err)

	var local int64
	dt.db.Model(&model.TeeNodeDatatableManagementDO{}).Count(&local)
	assert.Equal(t, before, local, "EDGE must not save TEE management records locally")

	require.Len(t, rec.items, 3)
	kinds := []string{}
	for i, it := range rec.items {
		assert.Equal(t, "", rec.node[i], "sent to the center")
		assert.Equal(t, "tee_node_datatable_management", it.TableName)
		assert.Equal(t, SyncActionCreate, it.Action)
		m, ok := it.Data.(*model.TeeNodeDatatableManagementDO)
		require.True(t, ok)
		assert.False(t, m.GmtCreate.IsZero())
		kinds = append(kinds, m.Kind)
		if m.Kind != TeeJobKindPushAuth {
			// the Kuscia job is still created from the pushed record
			assert.Contains(t, fk.jobs, m.JobID)
		}
	}
	assert.Equal(t, []string{TeeJobKindPushAuth, TeeJobKindPush, TeeJobKindPull}, kinds)

	// CENTER / AUTONOMY keep saving locally.
	SetGlobalEnv(NewEnvService("CENTER", "kuscia-system", "", nil))
	_, err = NewTeePullService(dt).PullResult(ctx, &TeePullRequest{NodeID: "alice", ResourceID: "r-2", ProjectID: "p1"})
	require.NoError(t, err)
	dt.db.Model(&model.TeeNodeDatatableManagementDO{}).Count(&local)
	assert.Equal(t, before+1, local)
	assert.Len(t, rec.items, 3)
}
