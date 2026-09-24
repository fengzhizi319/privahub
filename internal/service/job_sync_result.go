package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// syncResult mirrors JobManager.syncResult: for every party and output
// DomainData "{jobId}-{output}" write project_result and the per-kind table
// (fed_table + datatable schema / rule / model / report / read_data).
func (s *JobStatusSyncService) syncResult(ctx context.Context, task *model.ProjectJobTaskDO) error {
	return syncTaskResult(ctx, s.db, s.kuscia, s.log, task)
}

func syncTaskResult(ctx context.Context, db *gorm.DB, k GraphKusciaAPI, log *zap.Logger, task *model.ProjectJobTaskDO) error {
	if graphjob.NormalizeTaskStatus(task.Status) != graphjob.TaskSucceed || k == nil {
		return nil
	}
	parties := taskParties(task)
	if len(parties) == 0 {
		return nil
	}
	node := loadTaskNodeInfo(ctx, db, task, "")
	for _, output := range node.Outputs {
		ddID := graphjob.GenTaskOutputID(task.JobID, output)
		createDomainGrantByUnion(ctx, db, k, task, &node, parties, ddID)
		for _, party := range parties {
			dd, err := k.QueryDomainData(ctx, party, ddID)
			if err != nil || dd == nil {
				continue // not produced for this party
			}
			kind := graphjob.ResultKindFromDatatable(dd.Data.Type)
			if kind == "" {
				log.Warn("unsupported domain data type", zap.String("type", dd.Data.Type), zap.String("domaindata_id", ddID))
				continue
			}
			distData := dd.Data.Attributes["dist_data"]
			if err := upsertResult(ctx, db, &model.ProjectResultDO{
				ProjectID: task.ProjectID, Kind: kind, NodeID: party, RefID: ddID, JobID: task.JobID, TaskID: task.TaskID,
			}); err != nil {
				return err
			}
			switch kind {
			case graphjob.ResultFedTable:
				joins := make([]map[string]string, 0, len(parties))
				for _, p := range parties {
					joins = append(joins, map[string]string{"nodeId": p, "datatableId": ddID})
				}
				jb, _ := json.Marshal(joins)
				cfgs := make([]graphjob.ColumnConfig, 0, len(dd.Data.Columns))
				for _, c := range dd.Data.Columns {
					cfgs = append(cfgs, graphjob.ColumnConfig{ColName: c.Name, ColType: c.Type, ColComment: c.Comment})
				}
				if len(cfgs) == 0 {
					cfgs = parseIndividualSchema(distData)
				}
				if err := upsertBy(ctx, db, &model.ProjectDatatableDO{
					ProjectID: task.ProjectID, NodeID: party, DatatableID: ddID,
					TableConfigs: graphjob.ColumnConfigsJSON(cfgs), Source: "CREATED",
				}, "project_id = ? AND node_id = ? AND datatable_id = ?", task.ProjectID, party, ddID); err != nil {
					return err
				}
				if err := upsertBy(ctx, db, &model.ProjectFedTableDO{ProjectID: task.ProjectID, FedTableID: ddID, Joins: string(jb)},
					"project_id = ? AND fed_table_id = ?", task.ProjectID, ddID); err != nil {
					return err
				}
			case graphjob.ResultRule:
				if err := upsertBy(ctx, db, &model.ProjectRuleDO{ProjectID: task.ProjectID, RuleID: ddID},
					"project_id = ? AND rule_id = ?", task.ProjectID, ddID); err != nil {
					return err
				}
			case graphjob.ResultModel:
				if err := upsertBy(ctx, db, &model.ProjectModelDO{ProjectID: task.ProjectID, ModelID: ddID},
					"project_id = ? AND model_id = ?", task.ProjectID, ddID); err != nil {
					return err
				}
			case graphjob.ResultReport:
				if err := upsertBy(ctx, db, &model.ProjectReportDO{ProjectID: task.ProjectID, ReportID: ddID, Content: distData},
					"project_id = ? AND report_id = ?", task.ProjectID, ddID); err != nil {
					return err
				}
			case graphjob.ResultReadData:
				if err := syncReadData(ctx, db, task, &node, ddID, distData); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// syncReadData mirrors the READ_DATA branch of JobManager.syncResult.
func syncReadData(ctx context.Context, db *gorm.DB, task *model.ProjectJobTaskDO, node *graphjob.GraphNodeInfo, ddID, distData string) error {
	if len(node.Inputs) == 0 {
		return nil
	}
	input := node.Inputs[0]
	upstream := graphjob.UpstreamGraphNodeFromInput(input)
	ut, err := latestTask(ctx, db, task.ProjectID, upstream)
	if err != nil {
		return err
	}
	taskOutputID := input
	if ut != nil {
		taskOutputID = graphjob.GenTaskOutputID(ut.JobID, input)
	}
	var dist map[string]json.RawMessage
	_ = json.Unmarshal([]byte(distData), &dist)
	meta := string(dist["meta"])
	var metaObj map[string]any
	_ = json.Unmarshal(dist["meta"], &metaObj)
	modelHash, _ := metaObj["modelHash"].(string)
	var prev model.ProjectReadDataDO
	raw := meta
	if db.WithContext(ctx).Where("project_id = ? AND output_id = ?", task.ProjectID, taskOutputID).Order("id DESC").First(&prev).Error == nil {
		raw = prev.Raw
	}
	content, _ := json.Marshal([]string{raw, meta})
	short := task.TaskID
	if len(short) > 4 {
		short = short[:4]
	}
	return upsertBy(ctx, db, &model.ProjectReadDataDO{
		ProjectID: task.ProjectID, ReportID: ddID, OutputID: taskOutputID, Hash: modelHash,
		Task: short, GrapNodeID: task.GraphNodeID, Content: string(content), Raw: raw,
	}, "project_id = ? AND report_id = ?", task.ProjectID, ddID)
}

// createDomainGrantByUnion mirrors JobManager.createDomainGrantByUnion.
func createDomainGrantByUnion(ctx context.Context, db *gorm.DB, k GraphKusciaAPI, task *model.ProjectJobTaskDO,
	node *graphjob.GraphNodeInfo, parties []string, ddID string) {
	switch strings.ToLower(node.CodeName) {
	case graphjob.DataPrepUnion, graphjob.DataFilterExprCondition, graphjob.DataFilterSample:
	default:
		return
	}
	if len(parties) != 1 {
		return
	}
	var pns []model.ProjectNodeDO
	db.WithContext(ctx).Where("project_id = ?", task.ProjectID).Find(&pns)
	for _, pn := range pns {
		if pn.NodeID == parties[0] {
			continue
		}
		_ = grantDomainData(ctx, k, parties[0], pn.NodeID, ddID)
	}
}

// parseIndividualSchema mirrors JobManager.parse for sf.table.individual DistData.
func parseIndividualSchema(distData string) []graphjob.ColumnConfig {
	out := []graphjob.ColumnConfig{}
	if distData == "" {
		return out
	}
	var dd struct {
		Type     string                     `json:"type"`
		Meta     map[string]json.RawMessage `json:"meta"`
		DataRefs []struct {
			Party string `json:"party"`
		} `json:"data_refs"`
		DataRefsCamel []struct {
			Party string `json:"party"`
		} `json:"dataRefs"`
	}
	if json.Unmarshal([]byte(distData), &dd) != nil || dd.Type != "sf.table.individual" {
		return out
	}
	party := ""
	if len(dd.DataRefs) > 0 {
		party = dd.DataRefs[0].Party
	} else if len(dd.DataRefsCamel) > 0 {
		party = dd.DataRefsCamel[0].Party
	}
	var schema map[string][]string
	_ = json.Unmarshal(dd.Meta["schema"], &schema)
	get := func(keys ...string) []string {
		for _, k := range keys {
			if v, ok := schema[k]; ok {
				return v
			}
		}
		return nil
	}
	add := func(names, types []string) {
		for i, n := range names {
			t := ""
			if i < len(types) {
				t = types[i]
			}
			out = append(out, graphjob.ColumnConfig{ColName: n, ColType: t, ColComment: "individual:" + party})
		}
	}
	add(get("ids"), get("id_types", "idTypes"))
	add(get("features"), get("feature_types", "featureTypes"))
	add(get("labels"), get("label_types", "labelTypes"))
	return out
}

func upsertResult(ctx context.Context, db *gorm.DB, r *model.ProjectResultDO) error {
	return upsertBy(ctx, db, r, "project_id = ? AND kind = ? AND node_id = ? AND ref_id = ?", r.ProjectID, r.Kind, r.NodeID, r.RefID)
}

// upsertBy updates the row matching where (keeping its primary key / create
// time) or inserts a new one. Soft-deleted rows are revived.
func upsertBy[T any](ctx context.Context, db *gorm.DB, row *T, where string, args ...any) error {
	var existing T
	err := db.WithContext(ctx).Unscoped().Where(where, args...).First(&existing).Error
	if err == nil {
		setBase(row, baseOf(&existing))
		return db.WithContext(ctx).Unscoped().Save(row).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.WithContext(ctx).Create(row).Error
}

func baseOf(v any) model.BaseDO {
	switch t := v.(type) {
	case *model.ProjectResultDO:
		return t.BaseDO
	case *model.ProjectDatatableDO:
		return t.BaseDO
	case *model.ProjectFedTableDO:
		return t.BaseDO
	case *model.ProjectRuleDO:
		return t.BaseDO
	case *model.ProjectModelDO:
		return t.BaseDO
	case *model.ProjectReportDO:
		return t.BaseDO
	case *model.ProjectReadDataDO:
		return t.BaseDO
	}
	return model.BaseDO{}
}

func setBase(v any, b model.BaseDO) {
	b.DeletedAt = gorm.DeletedAt{}
	b.IsDeleted = 0
	switch t := v.(type) {
	case *model.ProjectResultDO:
		t.BaseDO = b
	case *model.ProjectDatatableDO:
		t.BaseDO = b
	case *model.ProjectFedTableDO:
		t.BaseDO = b
	case *model.ProjectRuleDO:
		t.BaseDO = b
	case *model.ProjectModelDO:
		t.BaseDO = b
	case *model.ProjectReportDO:
		t.BaseDO = b
	case *model.ProjectReadDataDO:
		t.BaseDO = b
	}
}
