package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

func teeEvalParam(t *testing.T, cfg string) (map[string]interface{}, map[string]interface{}) {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(cfg), &m))
	return m["sf_node_eval_param"].(map[string]interface{}), m["tee_task_config"].(map[string]interface{})
}

func TestTeePullService_PullAndSync(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	ctx := context.Background()
	puller := NewTeePullService(dt)

	err := puller.PullResultFromTeeNode(ctx, "alice", "abcd-g1-node-3-output-0", `{"vote":"ok"}`, "p1", "abcd", "abcd-g1-node-3", "table")
	require.NoError(t, err)
	var pull model.TeeNodeDatatableManagementDO
	require.NoError(t, dt.db.Where("kind = ?", TeeJobKindPull).First(&pull).Error)
	assert.Equal(t, "alice-abcd-g1-node-3-output-0", pull.DatatableID)
	assert.Equal(t, "tee", pull.TeeNodeID)
	assert.Equal(t, TeeJobStatusRunning, pull.Status)

	job := fk.jobs[pull.JobID]
	require.Len(t, job.Tasks, 1)
	assert.Equal(t, "tee-dm-image", job.Tasks[0].AppImage)
	require.Len(t, job.Tasks[0].Parties, 2)
	nep, _ := teeEvalParam(t, job.Tasks[0].InputConfig)
	assert.Equal(t, "data_management/download:0.0.1", nep["comp_id"])
	assert.Equal(t, []interface{}{"receiver/domain_id", "receiver/vote_result"}, nep["attr_paths"])
	attrs := nep["attrs"].([]interface{})
	assert.Equal(t, `{"vote":"ok"}`, attrs[1].(map[string]interface{})["s"])
	in := nep["inputs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "downloader_input", in["name"])
	assert.Equal(t, "dm://input?id=abcd-g1-node-3-output-0", in["data_refs"].([]interface{})[0].(map[string]interface{})["uri"])
	assert.Equal(t, "dm://output?datasource_id=default-data-source&id=alice-abcd-g1-node-3-output-0&uri=alice-abcd-g1-node-3-output-0",
		nep["output_uris"].([]interface{})[0])

	// running: result already visible with RUNNING pull status.
	dt.SyncTeeJobs(ctx)
	var res model.ProjectResultDO
	require.NoError(t, dt.db.Where("node_id = ? AND ref_id = ?", "alice", pull.DatatableID).First(&res).Error)
	assert.Equal(t, graphjob.ResultFedTable, res.Kind)
	assert.Equal(t, "p1", res.ProjectID)
	assert.Equal(t, "abcd", res.JobID)
	assert.Equal(t, "abcd-g1-node-3", res.TaskID)
	st := TeePullStatus(ctx, dt.db, "alice", "", pull.DatatableID)
	require.NotNil(t, st)
	assert.Equal(t, TeeJobStatusRunning, st.Status)

	fk.jobState[pull.JobID] = "Succeeded"
	dt.SyncTeeJobs(ctx)
	st = TeePullStatus(ctx, dt.db, "alice", "tee", pull.DatatableID)
	require.NotNil(t, st)
	assert.Equal(t, TeeJobStatusSuccess, st.Status)
	var cnt int64
	dt.db.Model(&model.ProjectResultDO{}).Where("ref_id = ?", pull.DatatableID).Count(&cnt)
	assert.Equal(t, int64(1), cnt)

	// validation
	assert.Equal(t, errcode.ParamError.Code, dataBizCode(puller.PullResultFromTeeNode(ctx, "", "x", "", "", "", "", "")))
}

func TestDatatableService_AuthDatatableToTee(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	seedTables(t, dt, fk)
	ctx := context.Background()
	_, err := dt.AuthDatatableToTee(ctx, "p1", "alice", "t2", "", "")
	assert.Equal(t, errcode.NodeRouteNotExistError.Code, dataBizCode(err))

	dt.db.Create(&model.NodeRouteDO{RouteID: "r1", SrcNodeID: "alice", DstNodeID: "tee"})
	dt.db.Create(&model.NodeRouteDO{RouteID: "r2", SrcNodeID: "tee", DstNodeID: "alice"})
	auth, err := dt.AuthDatatableToTee(ctx, "p1", "alice", "t2", "", "")
	require.NoError(t, err)
	assert.Equal(t, "tee-t2", auth.DatatableID)
	assert.Equal(t, TeeJobKindAuth, auth.Kind)
	// never pushed -> a push job was created too.
	var pushes int64
	dt.db.Model(&model.TeeNodeDatatableManagementDO{}).Where("kind = ? AND datatable_id = ?", TeeJobKindPush, "tee-t2").Count(&pushes)
	assert.Equal(t, int64(1), pushes)

	job := fk.jobs[auth.JobID]
	require.Len(t, job.Tasks, 1)
	require.Len(t, job.Tasks[0].Parties, 1)
	assert.Equal(t, "alice", job.Tasks[0].Parties[0].DomainID)
	nep, tc := teeEvalParam(t, job.Tasks[0].InputConfig)
	assert.Equal(t, "data_management/authorize:0.0.1", nep["comp_id"])
	assert.Equal(t, []interface{}{"owner/domain_id", "authorization_info/domain_ids", "authorization_info/root_certs",
		"authorization_info/columns", "authorization_info/project_id"}, nep["attr_paths"])
	attrs := nep["attrs"].([]interface{})
	assert.Equal(t, []interface{}{"kuscia-system"}, attrs[1].(map[string]interface{})["ss"])
	assert.Equal(t, []interface{}{"*"}, attrs[3].(map[string]interface{})["ss"])
	assert.Equal(t, "p1", attrs[4].(map[string]interface{})["s"])
	assert.Equal(t, "p1", tc["scope"])
	in := nep["inputs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "dm://input?id=tee-t2", in["data_refs"].([]interface{})[0].(map[string]interface{})["uri"])

	// pushed successfully -> no new push.
	dt.db.Model(&model.TeeNodeDatatableManagementDO{}).Where("kind = ?", TeeJobKindPush).Update("status", TeeJobStatusSuccess)
	_, err = dt.AuthDatatableToTee(ctx, "p1", "alice", "t2", "", "")
	require.NoError(t, err)
	dt.db.Model(&model.TeeNodeDatatableManagementDO{}).Where("kind = ? AND datatable_id = ?", TeeJobKindPush, "tee-t2").Count(&pushes)
	assert.Equal(t, int64(1), pushes)
}

type fakeTeeAuthorizer struct {
	calls []string
	err   error
}

func (f *fakeTeeAuthorizer) AuthDatatableToTee(_ context.Context, projectID, nodeID, datatableID, teeNodeID, datasourceID string) (*model.TeeNodeDatatableManagementDO, error) {
	f.calls = append(f.calls, projectID+"/"+nodeID+"/"+datatableID+"/"+teeNodeID+"/"+datasourceID)
	return &model.TeeNodeDatatableManagementDO{}, f.err
}

func TestProjectAddDatatable_TeeBranch(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.db.Model(&model.ProjectDO{}).Where("project_id = ?", "p1").Update("compute_mode", "TEE")
	f.k.addDomainData("alice", "t9", "table", nil, nil)
	req := &AddDatatableRequest{ProjectID: "p1", NodeID: "alice", DatatableID: "t9"}
	err := f.project.AddDatatable(adminCtx(), req)
	if pe, ok := AsPadError(err); !ok || pe.Code != errcode.DatatableCreateTeeJobFailed {
		t.Fatalf("want tee authorizer error, got %v", err)
	}
	fa := &fakeTeeAuthorizer{err: errors.New("boom")}
	f.project.SetTeeAuthorizer(fa)
	require.Error(t, f.project.AddDatatable(adminCtx(), req))
	var cnt int64
	f.db.Model(&model.ProjectDatatableDO{}).Where("datatable_id = ?", "t9").Count(&cnt)
	assert.Equal(t, int64(0), cnt)
	fa.err = nil
	require.NoError(t, f.project.AddDatatable(adminCtx(), req))
	assert.Equal(t, "p1/alice/t9//default-data-source", fa.calls[1])
	f.db.Model(&model.ProjectDatatableDO{}).Where("datatable_id = ?", "t9").Count(&cnt)
	assert.Equal(t, int64(1), cnt)
}
