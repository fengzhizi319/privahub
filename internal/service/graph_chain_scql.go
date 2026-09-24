package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// SCQL task input config (Java KusciaJobConverter.renderScqlInputConfig and
// scql/task_config.proto ScqlTaskInputConfig), rendered for stats/scql_analysis.

// SCQL CCL constraints (scql Constraint enum names, proto3 JSON).
const (
	ScqlConstraintPlaintext               = "PLAINTEXT"
	ScqlConstraintPlaintextAfterJoin      = "PLAINTEXT_AFTER_JOIN"
	ScqlConstraintPlaintextAfterGroupBy   = "PLAINTEXT_AFTER_GROUP_BY"
	ScqlConstraintPlaintextAfterAggregate = "PLAINTEXT_AFTER_AGGREGATE"

	scqlAttrScriptInput   = "script_input"
	scqlAttrTaskInitiator = "task_initiator"
	scqlCSVDBType         = "csvdb"
	jobIDLength           = 4
)

// ScqlColumn is TableMeta.Column.
type ScqlColumn struct {
	Name  string `json:"name,omitempty"`
	DType string `json:"dtype,omitempty"`
}

// ScqlTableMeta is scql TableMeta.
type ScqlTableMeta struct {
	TableName  string       `json:"table_name,omitempty"`
	RefTable   string       `json:"ref_table,omitempty"`
	DBType     string       `json:"db_type,omitempty"`
	TableOwner string       `json:"table_owner,omitempty"`
	Columns    []ScqlColumn `json:"columns,omitempty"`
}

// ScqlTableList is scql TableList.
type ScqlTableList struct {
	Tbls []ScqlTableMeta `json:"tbls,omitempty"`
}

// ScqlColumnDef is scql ColumnDef.
type ScqlColumnDef struct {
	ColumnName string `json:"column_name,omitempty"`
	TableName  string `json:"table_name,omitempty"`
}

// ScqlColumnControl is scql ColumnControl.
type ScqlColumnControl struct {
	Col        ScqlColumnDef `json:"col"`
	PartyCode  string        `json:"party_code,omitempty"`
	Constraint string        `json:"constraint,omitempty"`
}

// ScqlPrivacyPolicy is scql PrivacyPolicy.
type ScqlPrivacyPolicy struct {
	ColumnControlList []ScqlColumnControl `json:"column_control_list,omitempty"`
}

// ScqlTaskInputConfig is scql ScqlTaskInputConfig.
type ScqlTaskInputConfig struct {
	ProjectID string                       `json:"project_id,omitempty"`
	Query     string                       `json:"query,omitempty"`
	Tables    map[string]ScqlTableList     `json:"tables,omitempty"`
	Ccls      map[string]ScqlPrivacyPolicy `json:"ccls,omitempty"`
	OutputIDs map[string]string            `json:"output_ids,omitempty"`
	Initiator string                       `json:"initiator,omitempty"`
}

// ScqlConstraint mirrors buildColumnControl: the table owner sees every
// column in plaintext; other parties see unprotected columns in plaintext and
// protected ones after join (association key) / group by (group key) /
// aggregate.
func ScqlConstraint(c graphjob.ColumnConfig, party, tableOwner string) string {
	if party == tableOwner || !c.IsProtection {
		return ScqlConstraintPlaintext
	}
	switch {
	case c.IsAssociateKey:
		return ScqlConstraintPlaintextAfterJoin
	case c.IsGroupKey:
		return ScqlConstraintPlaintextAfterGroupBy
	default:
		return ScqlConstraintPlaintextAfterAggregate
	}
}

// scqlTableName mirrors KusciaJobConverter.getTableName.
func scqlTableName(input, nodeID string) string {
	if strings.Contains(input, "-") {
		rest := ""
		if len(input) > jobIDLength {
			rest = input[jobIDLength:]
		}
		return strings.ReplaceAll(nodeID+rest, "-", "_")
	}
	return strings.ReplaceAll(nodeID, "-", "_") + "_" + input
}

// scqlDBType mirrors KusciaJobConverter.getDbType.
func scqlDBType(datasourceID string) string {
	switch {
	case strings.HasPrefix(datasourceID, "mysql"):
		return "mysql"
	case strings.HasPrefix(datasourceID, "odps"):
		return "odps"
	}
	return scqlCSVDBType
}

// renderScqlInputConfig mirrors KusciaJobConverter.renderScqlInputConfig. The
// result receiver (task_initiator) is added to the task parties when absent.
func (s *GraphService) renderScqlInputConfig(ctx context.Context, job *projectJob, t *jobTask, nd *graphjob.NodeDef,
	dsCfg map[string]graphjob.DatasourceRef) (string, error) {
	edges := job.Edges
	tops := graphjob.FindTopNodes(edges, t.Node.GraphNodeID)
	topIDs := make([]string, 0, len(tops))
	for id := range tops {
		topIDs = append(topIDs, id)
	}
	sort.Strings(topIDs)
	byID := map[string]graphjob.GraphNodeInfo{}
	for _, n := range job.FullNodes {
		byID[n.GraphNodeID] = n
	}
	var tables []model.ProjectDatatableDO
	for _, id := range topIDs {
		n, ok := byID[id]
		if !ok {
			continue
		}
		dtID := n.Def().DatatableID()
		var dts []model.ProjectDatatableDO
		if err := s.db.WithContext(ctx).Where("project_id = ? AND datatable_id = ?", job.ProjectID, dtID).Order("id ASC").Find(&dts).Error; err != nil {
			return "", err
		}
		if len(dts) == 0 {
			return "", padErr(errcode.DatatableNotExists, dtID)
		}
		tables = append(tables, dts[0])
	}

	query, initiator := "", ""
	for i, p := range nd.AttrPaths {
		if i >= len(nd.Attrs) {
			break
		}
		var a struct {
			S  string   `json:"s"`
			SS []string `json:"ss"`
		}
		_ = json.Unmarshal(nd.Attrs[i], &a)
		switch p {
		case scqlAttrScriptInput:
			query = a.S
		case scqlAttrTaskInitiator:
			if len(a.SS) > 0 {
				initiator = a.SS[0]
			}
		}
	}
	if initiator != "" && !containsString(t.Parties, initiator) {
		t.Parties = append(t.Parties, initiator)
	}
	if len(t.Node.Inputs) == 0 {
		return "", padErr(errcode.GraphJobInvalid, "stats/scql_analysis has no input")
	}
	input := t.Node.Inputs[0]

	tableList := map[string]ScqlTableList{}
	ccls := map[string]ScqlPrivacyPolicy{}
	for _, dt := range tables {
		cfgs := graphjob.ParseColumnConfigs(dt.TableConfigs)
		tableName := scqlTableName(input, dt.NodeID)
		cols := make([]ScqlColumn, 0, len(cfgs))
		controls := make([]ScqlColumnControl, 0, len(cfgs)*len(t.Parties))
		for _, c := range cfgs {
			cols = append(cols, ScqlColumn{Name: c.ColName, DType: c.ColType})
			for _, party := range t.Parties {
				controls = append(controls, ScqlColumnControl{
					Col:        ScqlColumnDef{ColumnName: c.ColName, TableName: tableName},
					PartyCode:  party,
					Constraint: ScqlConstraint(c, party, dt.NodeID),
				})
			}
		}
		tableList[dt.NodeID] = ScqlTableList{Tbls: []ScqlTableMeta{{
			TableName:  tableName,
			TableOwner: dt.NodeID,
			RefTable:   input,
			DBType:     scqlDBType(dsCfg[dt.NodeID].ID),
			Columns:    cols,
		}}}
		ccls[dt.NodeID] = ScqlPrivacyPolicy{ColumnControlList: controls}
	}
	outputIDs := map[string]string{}
	if len(t.Node.Outputs) > 0 {
		outputIDs[initiator] = t.Node.Outputs[0]
	}
	cfg := ScqlTaskInputConfig{
		ProjectID: strings.ReplaceAll(t.TaskID, "-", ""),
		Query:     query,
		Tables:    tableList,
		Ccls:      ccls,
		OutputIDs: outputIDs,
		Initiator: initiator,
	}
	b, _ := json.Marshal(cfg)
	return string(b), nil
}
