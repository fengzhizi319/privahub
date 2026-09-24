package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"gorm.io/gorm"
)

// Graph node / job task output rendering (Java GraphServiceImpl.getGraphNodeTaskOutputVO).

// OutputResult mirrors GraphNodeOutputVO.OutputResult.
type OutputResult struct {
	Path           string `json:"path,omitempty"`
	NodeID         string `json:"nodeId"`
	NodeName       string `json:"nodeName,omitempty"`
	Type           string `json:"type,omitempty"`
	Fields         string `json:"fields,omitempty"`
	FieldTypes     string `json:"fieldTypes,omitempty"`
	TableID        string `json:"tableId,omitempty"`
	DsID           string `json:"dsId,omitempty"`
	DatasourceType string `json:"datasourceType,omitempty"`
}

// OutputFileMeta mirrors GraphNodeOutputVO.FileMeta.
type OutputFileMeta struct {
	Headers []map[string]string `json:"headers"`
	Rows    []OutputResult      `json:"rows"`
}

// GraphNodeOutputVO mirrors Java GraphNodeOutputVO.
type GraphNodeOutputVO struct {
	Type        string          `json:"type,omitempty"`
	CodeName    string          `json:"codeName,omitempty"`
	Tabs        any             `json:"tabs,omitempty"`
	Meta        *OutputFileMeta `json:"meta,omitempty"`
	JobID       string          `json:"jobId,omitempty"`
	TaskID      string          `json:"taskId,omitempty"`
	GraphID     string          `json:"graphID,omitempty"`
	Warning     []string        `json:"warning,omitempty"`
	GmtCreate   string          `json:"gmtCreate,omitempty"`
	GmtModified string          `json:"gmtModified,omitempty"`
}

type outputRenderer struct {
	db     *gorm.DB
	kuscia GraphKusciaAPI
	// channel optionally resolves the KusciaAPI of a node (AUTONOMY target node).
	channel func(nodeID string) GraphKusciaAPI
}

func (r *outputRenderer) kusciaFor(nodeID string) GraphKusciaAPI {
	if r.channel != nil {
		if k := r.channel(nodeID); k != nil {
			return k
		}
	}
	return r.kuscia
}

func newOutputRenderer(db *gorm.DB, k GraphKusciaAPI) *outputRenderer {
	return &outputRenderer{db: db, kuscia: k}
}

// jobGraphID returns a job's graph id from project_job or project_schedule_job.
func jobGraphID(ctx context.Context, db *gorm.DB, projectID, jobID string) (string, bool) {
	var j model.ProjectJobDO
	if db.WithContext(ctx).Where("project_id = ? AND job_id = ?", projectID, jobID).First(&j).Error == nil {
		return j.GraphID, true
	}
	var sj model.ProjectScheduleJobDO
	if db.WithContext(ctx).Where("job_id = ?", jobID).First(&sj).Error == nil {
		return sj.GraphID, true
	}
	return "", false
}

func (r *outputRenderer) taskOutput(ctx context.Context, t *model.ProjectJobTaskDO, outputID string) (*GraphNodeOutputVO, error) {
	projectID := t.ProjectID
	vo := &GraphNodeOutputVO{}
	rows := make([]OutputResult, 0)
	graphID, jobFound := jobGraphID(ctx, r.db, projectID, t.JobID)
	node := loadTaskNodeInfo(ctx, r.db, t, graphID)

	if node.IsPad() {
		nd := node.Def()
		dtID := nd.DatatableID()
		vo.CodeName = node.CodeName
		if node.CodeName == graphjob.CompReadModel || nd.CompID() == graphjob.CompReadModel {
			vo.Type = "model"
			var pack model.ProjectModelPackDO
			if r.db.WithContext(ctx).Where("model_id = ?", dtID).First(&pack).Error == nil {
				vo.GmtCreate, vo.GmtModified = rfc3339(pack.GmtCreate), rfc3339(pack.GmtModified)
				for _, pd := range parsePartyDataSources(pack.ModelDatasource, pack.Initiator) {
					rows = append(rows, OutputResult{NodeID: pd[0], Path: pd[1], Type: "model", TableID: dtID, DsID: pd[1], DatasourceType: pd[1]})
				}
			}
		} else {
			vo.Type = "table"
			var dts []model.ProjectDatatableDO
			r.db.WithContext(ctx).Where("project_id = ? AND datatable_id = ?", projectID, dtID).Order("id ASC").Find(&dts)
			for i := range dts {
				rows = append(rows, r.fromDatatable(ctx, &dts[i], ""))
			}
			if len(dts) > 0 {
				vo.GmtCreate, vo.GmtModified = rfc3339(dts[0].GmtCreate), rfc3339(dts[0].GmtModified)
			}
		}
	} else {
		vo.TaskID, vo.JobID = t.TaskID, t.JobID
		if !jobFound {
			return nil, padErr(errcode.ProjectJobNotExists)
		}
		vo.GraphID = graphID
		if len(node.Outputs) == 0 || containsString(node.Outputs, outputID) {
			latestOutputID := graphjob.GenTaskOutputID(t.JobID, outputID)
			var results []model.ProjectResultDO
			r.db.WithContext(ctx).Where("project_id = ? AND task_id = ? AND ref_id = ?", projectID, t.TaskID, latestOutputID).
				Order("id ASC").Find(&results)
			r.compensation(ctx, t, &node, outputID, vo)
			for _, res := range results {
				vo.Type = graphjob.OutputTypeFromResultKind(res.Kind)
				vo.CodeName = node.CodeName
				vo.GmtCreate, vo.GmtModified = rfc3339(res.GmtCreate), rfc3339(res.GmtModified)
				// AUTONOMY: results of other institutions are read through a
				// local project node that is not a task party (targetNodeId).
				target := autonomyTargetNode(ctx, r.db, projectID, res.NodeID, taskParties(t))
				dsID, dsType := r.resultDatasource(ctx, res.NodeID, res.RefID, target)
				switch res.Kind {
				case graphjob.ResultReport:
					if err := r.fillReport(ctx, projectID, latestOutputID, vo); err != nil {
						return nil, err
					}
					return vo, nil
				case graphjob.ResultReadData:
					var rd model.ProjectReadDataDO
					if r.db.WithContext(ctx).Where("project_id = ? AND report_id = ?", projectID, latestOutputID).First(&rd).Error != nil {
						return nil, padErr(errcode.GraphNodeOutputNotExists)
					}
					vo.Tabs = readDataTabs(rd.Content, vo.Tabs)
					return vo, nil
				case graphjob.ResultModel, graphjob.ResultRule:
					typ := "model"
					if res.Kind == graphjob.ResultRule {
						typ = "rule"
					}
					rows = append(rows, OutputResult{NodeID: res.NodeID, Path: latestOutputID, Type: typ, TableID: latestOutputID, DsID: dsID, DatasourceType: dsType})
				case graphjob.ResultFedTable:
					var dt model.ProjectDatatableDO
					if r.db.WithContext(ctx).Where("project_id = ? AND node_id = ? AND datatable_id = ?", projectID, res.NodeID, latestOutputID).First(&dt).Error != nil {
						return nil, padErr(errcode.DatatableNotExists)
					}
					row := r.fromDatatable(ctx, &dt, "")
					row.DatasourceType = dsType
					if row.DsID == "" {
						row.DsID = dsID
					}
					rows = append(rows, row)
				default:
					return nil, padErr(errcode.DatatableUnsupportedDatatableType)
				}
			}
		}
	}
	for i := range rows {
		rows[i].NodeName = nodeName(ctx, r.db, rows[i].NodeID)
	}
	vo.Meta = &OutputFileMeta{
		Headers: []map[string]string{{"name": "metas", "type": "AT_STRING"}},
		Rows:    rows,
	}
	return vo, nil
}

func containsString(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// parsePartyDataSources parses [{partyId, datasource}] (Java PartyDataSource).
func parsePartyDataSources(raw, initiator string) [][2]string {
	var items []map[string]string
	out := [][2]string{}
	if json.Unmarshal([]byte(raw), &items) == nil {
		for _, it := range items {
			p := it["partyId"]
			if p == "" {
				p = it["party_id"]
			}
			out = append(out, [2]string{p, it["datasource"]})
		}
		return out
	}
	if raw != "" {
		out = append(out, [2]string{initiator, raw})
	}
	return out
}

// resultDatasource resolves a result's datasource id / platform type via Kuscia.
func (r *outputRenderer) resultDatasource(ctx context.Context, nodeID, refID, targetNodeID string) (string, string) {
	if targetNodeID == "" {
		targetNodeID = nodeID
	}
	k := r.kuscia
	if targetNodeID != nodeID {
		k = r.kusciaFor(targetNodeID)
	}
	if k == nil {
		return "", ""
	}
	dd, err := k.QueryDomainData(ctx, nodeID, refID)
	if err != nil || dd == nil {
		return "", ""
	}
	dsID := dd.Data.DatasourceID
	if list, err := k.ListDomainDataSource(ctx, targetNodeID); err == nil {
		for _, ds := range list {
			if ds.DatasourceID == dsID {
				return dsID, kusciaDatasourceTypeToPlatform(ds.Type)
			}
		}
	}
	return dsID, ""
}

// fromDatatable mirrors GraphServiceImpl.fromDatatable.
func (r *outputRenderer) fromDatatable(ctx context.Context, dt *model.ProjectDatatableDO, edgeNodeID string) OutputResult {
	fields, types := []string{}, []string{}
	for _, c := range graphjob.ParseColumnConfigs(dt.TableConfigs) {
		// Center mode hides other parties' individual-table columns.
		if strings.HasPrefix(c.ColComment, "individual") && c.ColComment != "individual:"+edgeNodeID {
			continue
		}
		fields = append(fields, c.ColName)
		types = append(types, c.ColType)
	}
	nodeType := ""
	var n model.NodeDO
	if r.db.WithContext(ctx).Where("node_id = ?", dt.NodeID).First(&n).Error == nil {
		nodeType = n.Type
	}
	out := OutputResult{
		NodeID:     dt.NodeID,
		Type:       nodeType,
		Fields:     strings.Join(fields, ","),
		FieldTypes: strings.Join(types, ","),
		TableID:    dt.DatatableID,
	}
	if r.kuscia != nil {
		if dd, err := r.kuscia.QueryDomainData(ctx, dt.NodeID, dt.DatatableID); err == nil && dd != nil {
			out.Path = dd.Data.RelativeURI
			out.DsID = dd.Data.DatasourceID
		}
	}
	return out
}

// fillReport mirrors GraphServiceImpl.getGraphNodeOutputVO (report tabs).
func (r *outputRenderer) fillReport(ctx context.Context, projectID, reportID string, vo *GraphNodeOutputVO) error {
	var rep model.ProjectReportDO
	if r.db.WithContext(ctx).Where("project_id = ? AND report_id = ?", projectID, reportID).First(&rep).Error != nil {
		return padErr(errcode.GraphNodeOutputNotExists)
	}
	var content map[string]json.RawMessage
	tabs := any([]any{})
	if json.Unmarshal([]byte(rep.Content), &content) == nil {
		if metaRaw, ok := content["meta"]; ok {
			var meta map[string]json.RawMessage
			if json.Unmarshal(metaRaw, &meta) == nil {
				if t, ok := meta["tabs"]; ok && len(t) > 0 && string(t) != "null" {
					tabs = t
				}
			}
		}
	}
	vo.Tabs = tabs
	return nil
}

// readDataTabs mirrors the READ_DATA branch: content is a JSON array whose
// first element is replaced with the compensated tabs (when present).
func readDataTabs(content string, compensated any) any {
	var arr []json.RawMessage
	if json.Unmarshal([]byte(content), &arr) != nil {
		return content
	}
	if s, ok := compensated.(string); ok && s != "" && len(arr) > 0 {
		b, _ := json.Marshal(s)
		arr[0] = b
	}
	b, _ := json.Marshal(arr)
	return string(b)
}

// compensation mirrors compensationSecretPadComponent (binning / model param
// modification second output shows the raw read_data content).
func (r *outputRenderer) compensation(ctx context.Context, t *model.ProjectJobTaskDO, node *graphjob.GraphNodeInfo, outputID string, vo *GraphNodeOutputVO) {
	if outputID == "" || !strings.HasSuffix(outputID, "1") {
		return
	}
	if node.CodeName != graphjob.BinningModificationsCodeName && node.CodeName != graphjob.ModelParamModificationsCodeNm {
		return
	}
	if len(node.Inputs) == 0 {
		return
	}
	input := node.Inputs[0]
	upstream := graphjob.UpstreamGraphNodeFromInput(input)
	ut, err := latestTask(ctx, r.db, t.ProjectID, upstream)
	if err != nil || ut == nil {
		return
	}
	taskOutputID := graphjob.GenTaskOutputID(ut.JobID, input)
	var rd model.ProjectReadDataDO
	if r.db.WithContext(ctx).Where("project_id = ? AND output_id = ?", t.ProjectID, taskOutputID).Order("id DESC").First(&rd).Error == nil {
		vo.Tabs = rd.Raw
		vo.Type = "read_data"
		vo.CodeName = node.CodeName
	}
}
