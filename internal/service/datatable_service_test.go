package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func TestDatatableService_CreateLocalAndODPS(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	ctx := context.Background()
	vo, err := dt.CreateDatatableCompat(ctx, &CreateDatatableCompatRequest{
		OwnerID: "alice", NodeIDs: []string{"alice", "bob"}, DatatableName: "t1",
		DatasourceID: "default-data-source", DatasourceName: "default-data-source", DatasourceType: "LOCAL",
		RelativeURI: "t1.csv", Desc: "hello", NullStrs: []string{"NA"},
		Columns: []TableColumnCompat{{ColName: "id", ColType: "str", ColComment: "the id"}},
	})
	require.NoError(t, err)
	require.Len(t, vo.DataTableNodeInfos, 2)
	assert.Empty(t, vo.FailedCreatedNodes)
	id := vo.DataTableNodeInfos[0].DomainDataID
	assert.Len(t, id, 8)
	dd := fk.domaindata[vo.DataTableNodeInfos[0].NodeID][id]
	assert.Equal(t, "table", dd.Type)
	assert.Equal(t, kuscia.FileFormatCSV, dd.FileFormat)
	assert.Equal(t, "LOCAL", dd.Attributes["DatasourceType"])
	assert.Equal(t, "default-data-source", dd.Attributes["DatasourceName"])
	assert.Equal(t, `["NA"]`, dd.Attributes["NullStrs"])
	assert.Equal(t, "hello", dd.Attributes["description"])
	assert.Equal(t, "the id", dd.Columns[0].Comment)
	body := string(fk.lastBodies["/api/v1/domaindata/create"])
	assert.Contains(t, body, `"file_format":"CSV"`)
	assert.Contains(t, body, `"comment":"the id"`)
	assert.NotContains(t, body, `"author"`)

	// ODPS partition validation.
	_, err = dt.CreateDatatableCompat(ctx, &CreateDatatableCompatRequest{
		OwnerID: "alice", NodeIDs: []string{"alice"}, DatatableName: "t2", DatasourceID: "odps-x", DatasourceName: "o",
		DatasourceType: "ODPS", RelativeURI: "tbl", Columns: []TableColumnCompat{{ColName: "a", ColType: "int"}},
		Partition: &OdpsPartitionRequest{Type: "odps", Fields: []OdpsPartitionField{{Name: "dt-1", Type: "str"}}},
	})
	assert.Equal(t, errcode.ParamError.Code, dataBizCode(err))
	vo, err = dt.CreateDatatableCompat(ctx, &CreateDatatableCompatRequest{
		OwnerID: "alice", NodeIDs: []string{"alice"}, DatatableName: "t2", DatasourceID: "odps-x", DatasourceName: "o",
		DatasourceType: "ODPS", RelativeURI: "tbl", Columns: []TableColumnCompat{{ColName: "a", ColType: "int"}},
		Partition: &OdpsPartitionRequest{Type: "odps", Fields: []OdpsPartitionField{{Name: "dt", Type: "str"}}},
	})
	require.NoError(t, err)
	p := fk.domaindata["alice"][vo.DataTableNodeInfos[0].DomainDataID].Partition
	require.NotNil(t, p)
	assert.Equal(t, "dt", p.Fields[0].Name)

	// all nodes fail -> error
	fk.failDomains["alice"] = true
	_, err = dt.CreateDatatableCompat(ctx, &CreateDatatableCompatRequest{
		OwnerID: "alice", NodeIDs: []string{"alice"}, DatatableName: "t3", DatasourceID: "d", DatasourceName: "d",
		DatasourceType: "OSS", RelativeURI: "x.csv", Columns: []TableColumnCompat{{ColName: "a", ColType: "int"}},
	})
	assert.Equal(t, errcode.DatatableCreateFailed.Code, dataBizCode(err))
}

func TestDatatableService_HTTPCreateGetDelete(t *testing.T) {
	_, dt, _ := newTestDatasourceService(t)
	ctx := context.Background()
	_, err := dt.CreateDatatableCompat(ctx, &CreateDatatableCompatRequest{
		OwnerID: "alice", NodeIDs: []string{"alice"}, DatatableName: "feat", DatasourceType: "HTTP",
		RelativeURI: "http://127.0.0.1:1/feature", Columns: []TableColumnCompat{{ColName: "f", ColType: "float"}},
	})
	require.NoError(t, err)
	var ft model.FeatureTableDO
	require.NoError(t, dt.db.Where("feature_table_name = ?", "feat").First(&ft).Error)
	assert.Equal(t, StatusUnavailable, ft.Status)
	assert.Equal(t, DefaultHTTPDatasourceID, ft.DatasourceID)

	got, err := dt.GetDatatableCompat(ctx, &GetDatatableCompatRequest{NodeID: "alice", DatatableID: ft.FeatureTableID, DatasourceType: "HTTP"})
	require.NoError(t, err)
	assert.Equal(t, "HTTP", got.DatatableVO.Type)
	assert.Equal(t, "f", got.DatatableVO.Schema[0].ColName)
	assert.Equal(t, "Alice", got.NodeName)

	// authorized -> refuse delete
	dt.db.Create(&model.ProjectDO{ProjectID: "p1", Name: "P1", ComputeMode: "mpc"})
	dt.db.Create(&model.ProjectFeatureTableDO{ProjectID: "p1", NodeID: "alice", FeatureTableID: ft.FeatureTableID, TableConfigs: "[]", Source: "IMPORTED"})
	err = dt.DeleteDatatable(ctx, &DeleteDatatableRequest{NodeID: "alice", DatatableID: ft.FeatureTableID, DatasourceType: "HTTP"})
	assert.Equal(t, errcode.DatatableDuplicatedAuthorized.Code, dataBizCode(err))
	dt.db.Where("project_id = ?", "p1").Delete(&model.ProjectFeatureTableDO{})
	require.NoError(t, dt.DeleteDatatable(ctx, &DeleteDatatableRequest{NodeID: "alice", DatatableID: ft.FeatureTableID, Type: "HTTP"}))

	// blocked URL with default filter
	dt.WithIPFilter(NewIPFilter(nil))
	_, err = dt.CreateDatatableCompat(ctx, &CreateDatatableCompatRequest{
		OwnerID: "alice", NodeIDs: []string{"alice"}, DatatableName: "feat2", DatasourceType: "HTTP",
		RelativeURI: "http://127.0.0.1/feature", Columns: []TableColumnCompat{{ColName: "f", ColType: "float"}},
	})
	assert.Equal(t, errcode.FeatureTableIpFilter.Code, dataBizCode(err))
}

func seedTables(t *testing.T, dt *DatatableService, fk *dataFakeKuscia) {
	fk.domaindata["alice"] = map[string]kuscia.DomainData{
		"t1": {DomainDataID: "t1", Name: "users", Type: "table", DatasourceID: "default-data-source", RelativeURI: "u.csv",
			Author: "alice", Status: "Available", Attributes: map[string]string{"NullStrs": `["x"]`, "description": "d1"},
			Columns: []kuscia.DataColumn{{Name: "id", Type: "str", Comment: "c"}}},
		"t2": {DomainDataID: "t2", Name: "orders", Type: "table", DatasourceID: "oss-a", Author: "alice", Status: "Unavailable",
			Attributes: map[string]string{"DatasourceType": "OSS", "DatasourceName": "ossA"}},
	}
	cols, _ := json.Marshal([]TableColumnCompat{{ColName: "f", ColType: "float"}})
	dt.db.Create(&model.FeatureTableDO{FeatureTableID: "ft1", FeatureTableName: "feat", NodeID: "alice", URL: "http://x", Columns: string(cols), Status: StatusUnavailable, Type: "HTTP"})
	dt.db.Create(&model.ProjectDO{ProjectID: "p1", Name: "P1", ComputeMode: "mpc"})
	dt.db.Create(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "t1", Source: "IMPORTED",
		TableConfigs: `[{"colName":"id","isAssociateKey":true},{"colName":"y","isLabelKey":true}]`})
	dt.db.Create(&model.TeeNodeDatatableManagementDO{NodeID: "alice", TeeNodeID: "tee", DatatableID: "tee-t1", JobID: "abcd",
		DatasourceID: "default-data-source", Kind: TeeJobKindPush, Status: TeeJobStatusSuccess})
}

func TestDatatableService_ListFromKuscia(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	seedTables(t, dt, fk)
	ctx := context.Background()
	vo, err := dt.ListDatatablesByOwnerID(ctx, &ListDatatableRequest{OwnerID: "alice", PageSize: 10, PageNumber: 1})
	require.NoError(t, err)
	assert.Equal(t, 3, vo.TotalDatatableNums)
	byID := map[string]DatatableNodeVO{}
	for _, it := range vo.DatatableNodeVOList {
		byID[it.DatatableVO.DatatableID] = it
	}
	t1 := byID["t1"].DatatableVO
	assert.Equal(t, "CSV", t1.Type)
	assert.Equal(t, "LOCAL", t1.DatasourceType)
	assert.Equal(t, "default-data-source", t1.DatasourceName)
	assert.Equal(t, "d1", t1.Description)
	assert.Equal(t, []string{"x"}, t1.NullStrs)
	assert.Equal(t, "SUCCESS", t1.PushToTeeStatus)
	require.Len(t, t1.AuthProjects, 1)
	assert.Equal(t, "P1", t1.AuthProjects[0].Name)
	assert.Equal(t, []string{"id"}, t1.AuthProjects[0].AssociateKeys)
	assert.Equal(t, []string{"y"}, t1.AuthProjects[0].LabelKeys)
	assert.Equal(t, "Alice", byID["t1"].NodeName)
	assert.Equal(t, "HTTP", byID["ft1"].DatatableVO.Type)

	vo, err = dt.ListDatatablesByOwnerID(ctx, &ListDatatableRequest{OwnerID: "alice", StatusFilter: "unavailable", Types: []string{"OSS"}})
	require.NoError(t, err)
	require.Len(t, vo.DatatableNodeVOList, 1)
	assert.Equal(t, "t2", vo.DatatableNodeVOList[0].DatatableVO.DatatableID)

	name := "user"
	vo, err = dt.ListDatatablesByOwnerID(ctx, &ListDatatableRequest{OwnerID: "alice", DatatableNameFilter: &name})
	require.NoError(t, err)
	assert.Equal(t, 1, vo.TotalDatatableNums)

	vo, err = dt.ListDatatablesByOwnerID(ctx, &ListDatatableRequest{OwnerID: "alice", PageSize: 2, PageNumber: 2})
	require.NoError(t, err)
	assert.Len(t, vo.DatatableNodeVOList, 1)
	assert.Equal(t, 3, vo.TotalDatatableNums)

	// FindDatatablesByNodeID (datasource relatedDatas)
	list, err := dt.FindDatatablesByNodeID(ctx, "alice")
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestDatatableService_GetAndDelete(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	seedTables(t, dt, fk)
	ctx := context.Background()
	got, err := dt.GetDatatableCompat(ctx, &GetDatatableCompatRequest{NodeID: "alice", DatatableID: "t1"})
	require.NoError(t, err)
	assert.Equal(t, "users", got.DatatableVO.DatatableName)
	assert.Len(t, got.DatatableVO.AuthProjects, 1)

	_, err = dt.GetDatatableCompat(ctx, &GetDatatableCompatRequest{NodeID: "alice", DatatableID: "missing", DatasourceType: "LOCAL"})
	assert.Equal(t, errcode.DatatableNotExists.Code, dataBizCode(err))

	// authorized to a project -> refused, no project id needed
	err = dt.DeleteDatatable(ctx, &DeleteDatatableRequest{NodeID: "alice", DatatableID: "t1", DatasourceType: "LOCAL"})
	assert.Equal(t, errcode.DatatableDuplicatedAuthorized.Code, dataBizCode(err))

	// pushed to TEE (SUCCESS) -> TEE delete job, then Kuscia delete
	dt.db.Where("datatable_id = ?", "t1").Delete(&model.ProjectDatatableDO{})
	require.NoError(t, dt.DeleteDatatable(ctx, &DeleteDatatableRequest{NodeID: "alice", DatatableID: "t1", DatasourceType: "LOCAL"}))
	assert.NotContains(t, fk.domaindata["alice"], "t1")
	var del model.TeeNodeDatatableManagementDO
	require.NoError(t, dt.db.Where("kind = ?", TeeJobKindDelete).First(&del).Error)
	assert.Equal(t, TeeJobStatusRunning, del.Status)
	job, ok := fk.jobs[del.JobID]
	require.True(t, ok)
	assert.Contains(t, job.Tasks[0].InputConfig, "data_management/delete:0.0.1")
	assert.Equal(t, "alice", job.Initiator)

	// TEE job sync: delete job succeeded -> Push record soft deleted
	fk.jobState[del.JobID] = "Succeeded"
	dt.SyncTeeJobs(ctx)
	require.NoError(t, dt.db.Where("id = ?", del.ID).First(&del).Error)
	assert.Equal(t, TeeJobStatusSuccess, del.Status)
	var cnt int64
	dt.db.Model(&model.TeeNodeDatatableManagementDO{}).Where("job_id = ?", "abcd").Count(&cnt)
	assert.EqualValues(t, 0, cnt)

	// Kuscia delete of a table without TEE push
	require.NoError(t, dt.DeleteDatatable(ctx, &DeleteDatatableRequest{NodeID: "alice", DatatableID: "t2"}))
	assert.NotContains(t, fk.domaindata["alice"], "t2")
}

func TestDatatableService_PushToTee(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	seedTables(t, dt, fk)
	ctx := context.Background()
	req := &PushDatatableToTeeRequest{NodeID: "alice", DatatableID: "t2"}
	err := dt.PushDatatableToTee(ctx, req)
	assert.Equal(t, errcode.NodeRouteNotExistError.Code, dataBizCode(err))

	dt.db.Create(&model.NodeRouteDO{RouteID: "r1", SrcNodeID: "alice", DstNodeID: "tee"})
	dt.db.Create(&model.NodeRouteDO{RouteID: "r2", SrcNodeID: "tee", DstNodeID: "alice"})
	require.NoError(t, dt.PushDatatableToTee(ctx, req))
	assert.Len(t, fk.grants, 1)
	var push model.TeeNodeDatatableManagementDO
	require.NoError(t, dt.db.Where("kind = ? AND datatable_id = ?", TeeJobKindPush, "tee-t2").First(&push).Error)
	assert.Equal(t, TeeJobStatusRunning, push.Status)
	assert.Contains(t, push.OperateInfo, `"relativeUri":"tee-t2"`)
	job := fk.jobs[push.JobID]
	require.Len(t, job.Tasks, 1)
	assert.Equal(t, push.JobID+"-alice", job.Tasks[0].TaskID)
	assert.Equal(t, "tee-dm-image", job.Tasks[0].AppImage)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(job.Tasks[0].InputConfig), &cfg))
	nep := cfg["sf_node_eval_param"].(map[string]interface{})
	assert.Equal(t, "data_management/upload:0.0.1", nep["comp_id"])
	inputs := nep["inputs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "dm://input?id=t2", inputs["data_refs"].([]interface{})[0].(map[string]interface{})["uri"])
	assert.Equal(t, "dm://output?datasource_id=default-data-source&id=tee-t2&uri=tee-t2", nep["output_uris"].([]interface{})[0])
	assert.Equal(t, "capsule-manager.tee.svc", cfg["tee_task_config"].(map[string]interface{})["capsule_manager_endpoint"])
	assert.Contains(t, cfg["sf_datasource_config"], "tee")

	// second push reuses the grant
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, dt.PushDatatableToTee(ctx, req))
	assert.Len(t, fk.grants, 1)

	// list shows RUNNING push status for t2
	vo, err := dt.ListDatatablesByOwnerID(ctx, &ListDatatableRequest{OwnerID: "alice"})
	require.NoError(t, err)
	for _, it := range vo.DatatableNodeVOList {
		if it.DatatableVO.DatatableID == "t2" {
			assert.Equal(t, TeeJobStatusRunning, it.DatatableVO.PushToTeeStatus)
		}
	}
}

func TestTeeJobConverter_Render(t *testing.T) {
	c := NewTeeJobConverter("cm.#.svc", 3)
	m := &model.TeeNodeDatatableManagementDO{NodeID: "alice", TeeNodeID: "tee", DatatableID: "tee-t9", JobID: "wxyz",
		DatasourceID: "ds", Kind: TeeJobKindPush, OperateInfo: `{"relativeUri":""}`}
	req := c.Convert(GenTeeJob(m, []string{"alice", "tee"}, "", nil, nil))
	assert.EqualValues(t, 3, req.MaxParallelism)
	assert.True(t, strings.Contains(req.Tasks[0].InputConfig, `"output_uris":["dm://output?datasource_id=ds&id=tee-t9"]`) ||
		strings.Contains(req.Tasks[0].InputConfig, `"output_uris":["dm://output?datasource_id=ds&id=tee-t9"]`))
	assert.Contains(t, req.Tasks[0].InputConfig, `"capsule_manager_endpoint":"cm.tee.svc"`)
	assert.Equal(t, "tee-x", BuildTeeDatatableID("tee", "x"))
}

// A push / list / delete with a request teeNodeId different from the
// configured TEE node use the same TEE node for the record and the TEE-side
// datatable id, so the TEE copy is found (and deleted) again.
func TestDatatableService_TeeIDConsistentAcrossPushListDelete(t *testing.T) {
	_, dt, fk := newTestDatasourceService(t)
	seedTables(t, dt, fk)
	ctx := context.Background()
	dt.db.Create(&model.NodeRouteDO{RouteID: "r1", SrcNodeID: "alice", DstNodeID: "tee2"})
	dt.db.Create(&model.NodeRouteDO{RouteID: "r2", SrcNodeID: "tee2", DstNodeID: "alice"})

	require.NoError(t, dt.PushDatatableToTee(ctx, &PushDatatableToTeeRequest{NodeID: "alice", DatatableID: "t2", TeeNodeID: "tee2"}))
	var push model.TeeNodeDatatableManagementDO
	require.NoError(t, dt.db.Where("kind = ? AND tee_node_id = ?", TeeJobKindPush, "tee2").First(&push).Error)
	assert.Equal(t, BuildTeeDatatableID("tee2", "t2"), push.DatatableID)

	vo, err := dt.ListDatatablesByOwnerID(ctx, &ListDatatableRequest{OwnerID: "alice", TeeNodeID: "tee2"})
	require.NoError(t, err)
	found := false
	for _, it := range vo.DatatableNodeVOList {
		if it.DatatableVO.DatatableID == "t2" {
			found = true
			assert.Equal(t, TeeJobStatusRunning, it.DatatableVO.PushToTeeStatus)
		}
	}
	require.True(t, found)

	dt.db.Model(&model.TeeNodeDatatableManagementDO{}).Where("id = ?", push.ID).Update("status", TeeJobStatusSuccess)
	require.NoError(t, dt.DeleteDatatable(ctx, &DeleteDatatableRequest{NodeID: "alice", DatatableID: "t2", TeeNodeID: "tee2"}))
	var del model.TeeNodeDatatableManagementDO
	require.NoError(t, dt.db.Where("kind = ? AND tee_node_id = ?", TeeJobKindDelete, "tee2").First(&del).Error, "TEE copy must get a delete job")
	assert.Equal(t, push.DatatableID, del.DatatableID)
}
