package graphjob

import (
	"encoding/json"
	"strings"
)

// DatasourceRef is TaskConfig.DatasourceConfig.
type DatasourceRef struct {
	ID string `json:"id"`
}

// DeviceDesc is SFClusterDesc.DeviceDesc.
type DeviceDesc struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Parties []string `json:"parties"`
	Config  string   `json:"config"`
}

// ClusterDesc is SFClusterDesc.
type ClusterDesc struct {
	Parties      []string          `json:"parties"`
	Devices      []DeviceDesc      `json:"devices"`
	RayFedConfig map[string]string `json:"ray_fed_config"`
}

// NodeEvalParam is secretflow_spec.v1 NodeEvalParam (comp_id form, as
// consumed by secretflow.kuscia.entry of the bundled SecretFlow).
type NodeEvalParam struct {
	Version       string            `json:"version"`
	CompID        string            `json:"comp_id"`
	AttrPaths     []string          `json:"attr_paths"`
	Attrs         []json.RawMessage `json:"attrs"`
	CheckpointURI string            `json:"checkpoint_uri,omitempty"`
}

// ColumnAttr / TableAttr are TaskConfig.TableAttr (id/label/feature/binning).
type ColumnAttr struct {
	ColName string `json:"col_name"`
	ColType string `json:"col_type"`
}

// TableAttr is TaskConfig.TableAttr.
type TableAttr struct {
	TableID     string       `json:"table_id"`
	ColumnAttrs []ColumnAttr `json:"column_attrs"`
}

// TaskInputConfig is the task_input_config JSON (Java TaskConfig.TaskInputConfig).
type TaskInputConfig struct {
	DatasourceConfig    map[string]DatasourceRef `json:"sf_datasource_config"`
	ClusterDesc         ClusterDesc              `json:"sf_cluster_desc"`
	NodeEvalParam       NodeEvalParam            `json:"sf_node_eval_param"`
	InputIDs            []string                 `json:"sf_input_ids,omitempty"`
	InputPartitionsSpec []string                 `json:"sf_input_partitions_spec,omitempty"`
	OutputIDs           []string                 `json:"sf_output_ids,omitempty"`
	OutputURIs          []string                 `json:"sf_output_uris,omitempty"`
	TableAttrs          []TableAttr              `json:"table_attrs,omitempty"`
}

// BuildClusterDesc mirrors KusciaJobConverter.buildSfClusterDesc.
func BuildClusterDesc(s *Settings, parties []string) ClusterDesc {
	devices := make([]DeviceDesc, 0, len(s.Devices))
	for _, d := range s.Devices {
		name, typ := d.Name, d.Type
		if typ == "" {
			typ = name
		}
		if name == "" {
			name = typ
		}
		devices = append(devices, DeviceDesc{Name: name, Type: typ, Parties: parties, Config: d.Config})
	}
	backend := s.CrossSiloCommBackend
	if backend == "" {
		backend = "brpc_link"
	}
	return ClusterDesc{Parties: parties, Devices: devices, RayFedConfig: map[string]string{"cross_silo_comm_backend": backend}}
}

// ColumnConfig is one project_datatable.table_configs entry (Java TableColumnConfig).
type ColumnConfig struct {
	ColName        string `json:"colName"`
	ColType        string `json:"colType"`
	ColComment     string `json:"colComment"`
	IsAssociateKey bool   `json:"isAssociateKey"`
	IsGroupKey     bool   `json:"isGroupKey"`
	IsLabelKey     bool   `json:"isLabelKey"`
	IsProtection   bool   `json:"isProtection"`
}

// ParseColumnConfigs parses table_configs (tolerant of snake_case keys).
func ParseColumnConfigs(raw string) []ColumnConfig {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var items []map[string]any
	if json.Unmarshal([]byte(raw), &items) != nil {
		return nil
	}
	str := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok {
				return v
			}
		}
		return ""
	}
	bl := func(m map[string]any, keys ...string) bool {
		for _, k := range keys {
			if v, ok := m[k].(bool); ok {
				return v
			}
		}
		return false
	}
	out := make([]ColumnConfig, 0, len(items))
	for _, m := range items {
		out = append(out, ColumnConfig{
			ColName:        str(m, "colName", "col_name", "name"),
			ColType:        str(m, "colType", "col_type", "type"),
			ColComment:     str(m, "colComment", "col_comment", "comment"),
			IsAssociateKey: bl(m, "isAssociateKey", "is_associate_key", "associateKey", "associate_key"),
			IsGroupKey:     bl(m, "isGroupKey", "is_group_key", "groupKey", "group_key"),
			IsLabelKey:     bl(m, "isLabelKey", "is_label_key", "labelKey", "label_key"),
			IsProtection:   bl(m, "isProtection", "is_protection", "protection"),
		})
	}
	return out
}

// ColumnConfigsJSON marshals column configs.
func ColumnConfigsJSON(cfgs []ColumnConfig) string {
	if cfgs == nil {
		cfgs = []ColumnConfig{}
	}
	b, _ := json.Marshal(cfgs)
	return string(b)
}

// ColumnAttrType mirrors GraphServiceImpl.parse (id / binning / label / feature).
func ColumnAttrType(c ColumnConfig) string {
	switch {
	case c.IsAssociateKey:
		return "id"
	case !c.IsGroupKey:
		if c.IsProtection {
			return "label"
		}
		return "feature"
	default:
		return "binning"
	}
}

// BuildTableAttr converts a datatable's column configs to a TableAttr.
func BuildTableAttr(tableID string, cfgs []ColumnConfig) TableAttr {
	attrs := make([]ColumnAttr, 0, len(cfgs))
	for _, c := range cfgs {
		attrs = append(attrs, ColumnAttr{ColName: c.ColName, ColType: ColumnAttrType(c)})
	}
	return TableAttr{TableID: tableID, ColumnAttrs: attrs}
}

// OutputURI converts an output DomainData id to its storage URI ('-' -> '_'),
// with the schedule date suffix in scheduled mode.
func OutputURI(outputID, scheduleDate string) string {
	u := strings.ReplaceAll(outputID, "-", "_")
	if scheduleDate != "" {
		u += "_" + scheduleDate
	}
	return u
}

// BuildEvalParam builds the NodeEvalParam for a node definition.
func BuildEvalParam(nd *NodeDef, checkpointURI string) NodeEvalParam {
	version := nd.Version
	attrPaths := nd.AttrPaths
	if attrPaths == nil {
		attrPaths = []string{}
	}
	attrs := nd.Attrs
	if attrs == nil {
		attrs = []json.RawMessage{}
	}
	return NodeEvalParam{
		Version:       "1.0.0",
		CompID:        nd.Domain + "/" + nd.Name + ":" + version,
		AttrPaths:     attrPaths,
		Attrs:         attrs,
		CheckpointURI: checkpointURI,
	}
}

// Marshal renders the task_input_config JSON string.
func (c *TaskInputConfig) Marshal() string {
	if c.DatasourceConfig == nil {
		c.DatasourceConfig = map[string]DatasourceRef{}
	}
	// Only send partition specs when at least one input is partitioned.
	has := false
	for _, p := range c.InputPartitionsSpec {
		if p != "" {
			has = true
			break
		}
	}
	if !has {
		c.InputPartitionsSpec = nil
	}
	b, _ := json.Marshal(c)
	return string(b)
}
