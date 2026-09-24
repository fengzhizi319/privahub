package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// JobRenderHandler (Java graph/chain/JobRenderHandler + NodeDefAdapter).

type jobRenderHandler struct{ s *GraphService }

func (h *jobRenderHandler) handle(ctx context.Context, cc *chainContext, job *projectJob) (*projectJob, error) {
	nj := job.clone()
	if err := h.renderInputs(ctx, cc, nj); err != nil {
		return nil, err
	}
	renderOutputs(nj)
	if err := h.renderCustomizeSpec(ctx, nj); err != nil {
		return nil, err
	}
	pruneJob(nj)
	return nj, nil
}

// renderInputs mirrors JobRenderHandler.renderInputs.
func (h *jobRenderHandler) renderInputs(ctx context.Context, cc *chainContext, job *projectJob) error {
	db := h.s.db
	builder := graphjob.NewGraphBuilder(job.FullNodes)
	selected := map[string]bool{}
	for _, t := range job.Tasks {
		selected[t.Node.GraphNodeID] = true
	}
	for _, t := range job.Tasks {
		node := &t.Node
		if node.IsPad() {
			if node.Def().DatatableID() == "" {
				return padErr(errcode.DatatableNotExists, node.GraphNodeID)
			}
			continue
		}
		nd := node.Def()
		nd.CoverCustomAttrs()
		deps := []string{}
		newInputs := []string{}
		for _, input := range node.Inputs {
			dep, ok := builder.NodeByInput(input)
			if !ok {
				return padErr(errcode.GraphNodeNotExists, input)
			}
			depID := dep.GraphNodeID
			if dep.IsPad() {
				if !selected[depID] {
					lt, err := latestTask(ctx, db, job.ProjectID, depID)
					if err != nil {
						return err
					}
					if lt == nil {
						return padErr(errcode.GraphDependentNodeNotRun, depID)
					}
				}
				dtID := dep.Def().DatatableID()
				if dep.CodeName == graphjob.CompReadDataDatatable || dep.Def().CompID() == graphjob.CompReadDataDatatable {
					if err := h.checkReadData(ctx, cc, job.ProjectID, &dep, dtID, t.Parties); err != nil {
						return err
					}
				}
				if cc.teeNodeID != "" {
					newInputs = append(newInputs, cc.teeNodeID+"-"+dtID)
				} else {
					newInputs = append(newInputs, dtID)
				}
				continue
			}
			if selected[depID] {
				newInputs = append(newInputs, input)
				deps = append(deps, graphjob.GenTaskID(job.JobID, depID))
				continue
			}
			// Unselected upstream SF component: reuse its latest task outputs.
			lt, err := latestTask(ctx, db, job.ProjectID, depID)
			if err != nil {
				return err
			}
			if lt == nil {
				return padErr(errcode.GraphDependentNodeNotRun, depID)
			}
			newInputs = append(newInputs, graphjob.GenTaskOutputID(lt.JobID, input))
		}
		node.NodeDef = json.RawMessage(nd.String())
		node.Inputs = newInputs
		t.Dependencies = deps
	}
	return nil
}

// checkReadData validates a read_data datatable against project_datatable and
// Kuscia (Java: datatableManager.findById per owner) and records its
// partition info for sf_input_partitions_spec.
func (h *jobRenderHandler) checkReadData(ctx context.Context, cc *chainContext, projectID string, dep *graphjob.GraphNodeInfo, dtID string, taskParties []string) error {
	var dts []model.ProjectDatatableDO
	if err := h.s.db.WithContext(ctx).Where("project_id = ? AND datatable_id = ?", projectID, dtID).Order("id ASC").Find(&dts).Error; err != nil {
		return err
	}
	if len(dts) == 0 {
		return padErr(errcode.DatatableNotExists, dtID)
	}
	tableName := ""
	columns := map[string]bool{}
	// P2P: the grant is queried on a project node of the local institution
	// (Java envService.findLocalNodeId(task)).
	localNode := ""
	if GlobalEnv().IsAutonomy() {
		localNode = h.s.findLocalNodeID(ctx, projectID, taskParties)
	}
	for _, dt := range dts {
		if h.s.kuscia == nil {
			continue
		}
		nodeID, k := dt.NodeID, h.s.kuscia
		if localNode != "" {
			nodeID, k = localNode, h.s.kusciaFor(localNode)
		}
		resp, err := k.QueryDomainData(ctx, nodeID, dtID)
		if err != nil || resp == nil {
			return padErr(errcode.DatatableNotExists, "nodeId="+nodeID, "tableId="+dtID)
		}
		tableName = resp.Data.RelativeURI
		// Partition columns of the (last) ODPS DomainData partition spec.
		columns = PartitionColumnsFromDomainData(&resp.Data)
	}
	cc.partitions[dtID] = partitionInfo{
		ReadRule:         dep.Def().DatatablePartition(),
		TableName:        tableName,
		PartitionColumns: columns,
	}
	return nil
}

// renderOutputs mirrors JobRenderHandler.renderOutputs.
func renderOutputs(job *projectJob) {
	outputMap := map[string]string{}
	for _, t := range job.Tasks {
		if len(t.Node.Outputs) == 0 {
			continue
		}
		outs := make([]string, 0, len(t.Node.Outputs))
		for _, o := range t.Node.Outputs {
			n := graphjob.GenTaskOutputID(job.JobID, o)
			outputMap[o] = n
			outs = append(outs, n)
		}
		t.Node.Outputs = outs
	}
	for _, t := range job.Tasks {
		for i, in := range t.Node.Inputs {
			if v, ok := outputMap[in]; ok {
				t.Node.Inputs[i] = v
			}
		}
	}
}

// pruneJob mirrors JobRenderHandler.pruneJob (drop platform components).
func pruneJob(job *projectJob) {
	kept := make([]*jobTask, 0, len(job.Tasks))
	for _, t := range job.Tasks {
		if !t.Node.IsPad() {
			kept = append(kept, t)
		}
	}
	job.Tasks = kept
}

// renderCustomizeSpec mirrors JobRenderHandler.renderCustomizeSpec: NodeDef
// adapters may split a task (the extra task is inserted before the original).
func (h *jobRenderHandler) renderCustomizeSpec(ctx context.Context, job *projectJob) error {
	out := make([]*jobTask, 0, len(job.Tasks))
	for _, t := range job.Tasks {
		nd := t.Node.Def()
		var writeType string
		switch nd.Name {
		case graphjob.BinningModifications:
			writeType = "sf.rule.binning"
		case graphjob.ModelParamModifications:
			writeType = "sf.model.ss_glm"
		}
		if writeType == "" {
			out = append(out, t)
			continue
		}
		extra, err := h.modificationsAdapter(ctx, nd, t, writeType)
		if err != nil {
			return err
		}
		if extra != nil {
			out = append(out, extra)
		}
		out = append(out, t)
	}
	job.Tasks = out
	return nil
}

// modificationsAdapter mirrors BinningModificationsAdapter /
// ModelParamModificationsAdapter: the original task becomes io/read_data
// (outputs[0] -> outputs[1]) and a new task "{taskId}--1" writes the edited
// rule/model (io/write_data) or passes it through (io/identity).
func (h *jobRenderHandler) modificationsAdapter(ctx context.Context, nd *graphjob.NodeDef, t *jobTask, writeType string) (*jobTask, error) {
	outputs := append([]string(nil), t.Node.Outputs...)
	if len(outputs) < 2 {
		return nil, padErr(errcode.GraphJobInvalid, t.Node.GraphNodeID)
	}
	origNode := t.Node.Clone()
	origDeps := t.Dependencies

	readDef := &graphjob.NodeDef{Domain: "io", Name: "read_data", Version: "1.0.0", AttrPaths: []string{}, Attrs: []json.RawMessage{}}
	readNode := origNode.Clone()
	readNode.NodeDef = json.RawMessage(readDef.String())
	readNode.Inputs = []string{outputs[0]}
	readNode.Outputs = []string{outputs[1]}
	t.Node = readNode

	newTaskID := graphjob.GenExtendTaskID(t.TaskID, "1")
	extra := &jobTask{TaskID: newTaskID, Parties: append([]string(nil), t.Parties...), Dependencies: origDeps}
	t.Dependencies = []string{newTaskID}

	if len(nd.Attrs) == 0 {
		idDef := &graphjob.NodeDef{Domain: "io", Name: "identity", Version: "1.0.0", AttrPaths: []string{}, Attrs: []json.RawMessage{}}
		n := origNode.Clone()
		n.NodeDef = json.RawMessage(idDef.String())
		n.Outputs = []string{outputs[0]}
		extra.Node = n
		return extra, nil
	}

	wd := nd.Clone()
	wd.Domain, wd.Name, wd.Version = "io", "write_data", "1.0.0"
	wd.AttrPaths = []string{"write_data", "write_data_type"}
	typeAttr, _ := json.Marshal(map[string]string{"s": writeType})
	wd.Attrs = append(wd.Attrs, typeAttr)

	// attrs[0].s holds the edited payload with its modelHash.
	var first struct {
		S string `json:"s"`
	}
	_ = json.Unmarshal(wd.Attrs[0], &first)
	payload := map[string]any{}
	_ = json.Unmarshal([]byte(first.S), &payload)
	modelHash, _ := payload["modelHash"].(string)
	graphNodeID := origNode.GraphNodeID
	if i := strings.Index(modelHash, "_"); i >= 0 {
		graphNodeID = modelHash[i+1:]
		modelHash = modelHash[:i]
		payload["modelHash"] = modelHash
		b, _ := json.Marshal(payload)
		a0, _ := json.Marshal(map[string]string{"s": string(b)})
		wd.Attrs[0] = a0
	}
	newInput := outputs[0]
	var rd model.ProjectReadDataDO
	if modelHash != "" && h.s.db.WithContext(ctx).Where("hash = ? AND grap_node_id = ?", modelHash, graphNodeID).First(&rd).Error == nil {
		id := rd.ReportID
		if i := strings.LastIndex(id, "-"); i > 0 {
			id = id[:i]
		}
		newInput = id + "-0"
	}
	n := origNode.Clone()
	n.NodeDef = json.RawMessage(wd.String())
	n.Inputs = []string{newInput}
	n.Outputs = []string{outputs[0]}
	extra.Node = n
	return extra, nil
}
