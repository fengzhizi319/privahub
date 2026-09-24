package graphjob

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
)

// Component code names / constants (Java ComponentConstants).
const (
	CompReadDataDatatable = "read_data/datatable"
	CompReadModel         = "ml.predict/read_model"

	BinningModifications          = "binning_modifications"
	BinningModificationsCodeName  = "feature/binning_modifications"
	ModelParamModifications       = "model_param_modifications"
	ModelParamModificationsCodeNm = "preprocessing/model_param_modifications"

	DataPrepUnion              = "data_prep/union"
	DataFilterSample           = "data_filter/sample"
	DataFilterExprCondition    = "data_filter/expr_condition_filter"
	DataPrepUnbalancePSICache  = "data_prep/unbalance_psi_cache"
	StatsSCQLAnalysis          = "stats/scql_analysis"
	CheckpointPrefix           = "ck"
	DefaultDataSourceID        = "default-data-source"
	customProtobufCls          = "custom_protobuf_cls"
	customValue                = "custom_value"
	partitionDelimiter         = ","
	domainDataPartitionDelimit = ";"
)

// PadComponents are components executed by the platform itself (no Kuscia task).
var PadComponents = map[string]bool{
	CompReadDataDatatable: true,
	CompReadModel:         true,
}

// NodeDef mirrors the Pipeline.NodeDef JSON kept on graph nodes.
type NodeDef struct {
	Domain        string            `json:"domain"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	AttrPaths     []string          `json:"attrPaths"`
	Attrs         []json.RawMessage `json:"attrs"`
	CheckpointURI string            `json:"checkpointUri,omitempty"`
}

// ParseNodeDef parses a stored or request nodeDef (string or object JSON).
func ParseNodeDef(raw string) *NodeDef {
	nd := &NodeDef{}
	if strings.TrimSpace(raw) == "" {
		return nd
	}
	var alt struct {
		NodeDef
		AttrPathsSnake []string `json:"attr_paths"`
		CheckpointSn   string   `json:"checkpoint_uri"`
	}
	if json.Unmarshal([]byte(raw), &alt) != nil {
		return nd
	}
	*nd = alt.NodeDef
	if len(nd.AttrPaths) == 0 {
		nd.AttrPaths = alt.AttrPathsSnake
	}
	if nd.CheckpointURI == "" {
		nd.CheckpointURI = alt.CheckpointSn
	}
	return nd
}

// Clone deep-copies the node definition.
func (n *NodeDef) Clone() *NodeDef {
	c := *n
	c.AttrPaths = append([]string(nil), n.AttrPaths...)
	c.Attrs = make([]json.RawMessage, len(n.Attrs))
	for i, a := range n.Attrs {
		c.Attrs[i] = append(json.RawMessage(nil), a...)
	}
	return &c
}

// CompID returns "domain/name".
func (n *NodeDef) CompID() string { return n.Domain + "/" + n.Name }

// String marshals the node definition.
func (n *NodeDef) String() string {
	b, _ := json.Marshal(n)
	return string(b)
}

func attrField(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

func attrString(raw json.RawMessage, key string) string {
	m := attrField(raw)
	if m == nil {
		return ""
	}
	var s string
	if v, ok := m[key]; ok && json.Unmarshal(v, &s) == nil {
		return s
	}
	return ""
}

func attrStrings(raw json.RawMessage, key string) []string {
	m := attrField(raw)
	if m == nil {
		return nil
	}
	var ss []string
	if v, ok := m[key]; ok && json.Unmarshal(v, &ss) == nil {
		return ss
	}
	return nil
}

// DatatableID mirrors ComponentTools.getDataTableId (attrs[0].s), preferring
// the "datatable_selected" attr path when present.
func (n *NodeDef) DatatableID() string {
	for i, p := range n.AttrPaths {
		if p == "datatable_selected" && i < len(n.Attrs) {
			if s := attrString(n.Attrs[i], "s"); s != "" {
				return s
			}
		}
	}
	if len(n.Attrs) > 0 {
		return attrString(n.Attrs[0], "s")
	}
	return ""
}

// DatatablePartition mirrors ComponentTools.getDataTablePartition.
func (n *NodeDef) DatatablePartition() string {
	for i, p := range n.AttrPaths {
		if p == "datatable_partition" && i < len(n.Attrs) {
			return attrString(n.Attrs[i], "s")
		}
	}
	if len(n.Attrs) == 2 {
		return attrString(n.Attrs[1], "s")
	}
	return ""
}

// HiddenPartyID mirrors ComponentTools.getHiddenPartyId (attrs[1].ss[0]).
func (n *NodeDef) HiddenPartyID() string {
	if len(n.Attrs) > 1 {
		if ss := attrStrings(n.Attrs[1], "ss"); len(ss) > 0 {
			return ss[0]
		}
	}
	return ""
}

// CoverCustomAttrs mirrors ComponentTools.coverAttrByCustomAttr: attributes
// carrying custom_protobuf_cls are replaced with {"s": json(custom_value)}.
func (n *NodeDef) CoverCustomAttrs() {
	for i, a := range n.Attrs {
		m := attrField(a)
		if m == nil {
			continue
		}
		if _, ok := m[customProtobufCls]; !ok {
			continue
		}
		v := m[customValue]
		var buf bytes.Buffer
		if len(v) > 0 && json.Compact(&buf, v) == nil {
			v = buf.Bytes()
		}
		s, _ := json.Marshal(map[string]string{"s": string(v)})
		n.Attrs[i] = s
	}
}

// GraphNodeInfo mirrors Java GraphNodeInfo (the frontend graph node contract).
type GraphNodeInfo struct {
	CodeName    string          `json:"codeName"`
	GraphNodeID string          `json:"graphNodeId"`
	Label       string          `json:"label"`
	X           int             `json:"x"`
	Y           int             `json:"y"`
	Inputs      []string        `json:"inputs"`
	Outputs     []string        `json:"outputs"`
	NodeDef     json.RawMessage `json:"nodeDef"`
}

// Def parses the node's nodeDef.
func (g *GraphNodeInfo) Def() *NodeDef { return ParseNodeDef(string(g.NodeDef)) }

// IsPad reports a platform (secretpad) component, judged on the nodeDef
// domain/name like Java isSecretpadComponent, with codeName as fallback.
func (g *GraphNodeInfo) IsPad() bool {
	nd := g.Def()
	if nd.Domain != "" || nd.Name != "" {
		return PadComponents[nd.CompID()]
	}
	return PadComponents[g.CodeName]
}

// Clone deep-copies the node info.
func (g GraphNodeInfo) Clone() GraphNodeInfo {
	c := g
	c.Inputs = append([]string(nil), g.Inputs...)
	c.Outputs = append([]string(nil), g.Outputs...)
	c.NodeDef = append(json.RawMessage(nil), g.NodeDef...)
	return c
}

// GraphEdge mirrors Java GraphEdge.
type GraphEdge struct {
	EdgeID       string `json:"edgeId"`
	Source       string `json:"source"`
	SourceAnchor string `json:"sourceAnchor"`
	Target       string `json:"target"`
	TargetAnchor string `json:"targetAnchor"`
}

// ParseEdges parses a stored edges JSON array (tolerating snake_case keys).
func ParseEdges(raw string) []GraphEdge {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var items []map[string]any
	if json.Unmarshal([]byte(raw), &items) != nil {
		return nil
	}
	get := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	out := make([]GraphEdge, 0, len(items))
	for _, m := range items {
		out = append(out, GraphEdge{
			EdgeID:       get(m, "edgeId", "edge_id"),
			Source:       get(m, "source"),
			SourceAnchor: get(m, "sourceAnchor", "source_anchor"),
			Target:       get(m, "target"),
			TargetAnchor: get(m, "targetAnchor", "target_anchor"),
		})
	}
	return out
}

// EdgesJSON marshals edges to the stored JSON form.
func EdgesJSON(edges []GraphEdge) string {
	if edges == nil {
		edges = []GraphEdge{}
	}
	b, _ := json.Marshal(edges)
	return string(b)
}

// FindTopNodes mirrors GraphUtils.findTopNodes: the root ancestors of nodeID.
func FindTopNodes(edges []GraphEdge, nodeID string) map[string]bool {
	incoming := map[string][]string{}
	for _, e := range edges {
		incoming[e.Target] = append(incoming[e.Target], e.Source)
	}
	result := map[string]bool{}
	visited := map[string]bool{}
	stack := []string{nodeID}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		srcs, ok := incoming[cur]
		if !ok {
			result[cur] = true
			continue
		}
		stack = append(stack, srcs...)
	}
	return result
}

// GraphBuilder mirrors Java GraphBuilder: resolves an input anchor to the
// node producing it.
type GraphBuilder struct {
	output2node map[string]string
	nodes       map[string]GraphNodeInfo
}

// NewGraphBuilder indexes graph nodes by output anchors.
func NewGraphBuilder(nodes []GraphNodeInfo) *GraphBuilder {
	b := &GraphBuilder{output2node: map[string]string{}, nodes: map[string]GraphNodeInfo{}}
	for _, n := range nodes {
		for _, o := range n.Outputs {
			b.output2node[o] = n.GraphNodeID
		}
		b.nodes[n.GraphNodeID] = n
	}
	return b
}

// NodeByInput returns the node producing inputID.
func (b *GraphBuilder) NodeByInput(inputID string) (GraphNodeInfo, bool) {
	id, ok := b.output2node[inputID]
	if !ok {
		// Fallback: anchors are "{graphNodeId}-output-{i}".
		if i := strings.LastIndex(inputID, "-output-"); i > 0 {
			id = inputID[:i]
		}
	}
	n, ok := b.nodes[id]
	return n, ok
}

// GenTaskID mirrors JobUtils.genTaskId.
func GenTaskID(jobID, graphNodeID string) string { return jobID + "-" + graphNodeID }

// GenTaskOutputID mirrors JobUtils.genTaskOutputId.
func GenTaskOutputID(jobID, outputID string) string { return jobID + "-" + outputID }

// GenExtendTaskID mirrors JobUtils.genExtendTaskId.
func GenExtendTaskID(taskID, extend string) string { return taskID + "--" + extend }

// RandomLetters mirrors UUIDUtils.random(n): lowercase letters only, which
// keeps ids valid DNS-1123 labels for Kuscia.
func RandomLetters(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

// UpstreamGraphNodeFromInput mirrors the Java trick of stripping the last two
// "-" segments of an input anchor ("{node}-output-0" -> "{node}").
func UpstreamGraphNodeFromInput(input string) string {
	s := input
	for k := 0; k < 2; k++ {
		i := strings.LastIndex(s, "-")
		if i <= 0 {
			return s
		}
		s = s[:i]
	}
	return s
}

// PartitionRuleToDomainData converts "a,b" partition rules to Kuscia's "a;b".
func PartitionRuleToDomainData(rule string) string {
	return strings.ReplaceAll(rule, partitionDelimiter, domainDataPartitionDelimit)
}
