package graphjob

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNodeDefHelpers(t *testing.T) {
	nd := ParseNodeDef(`{"domain":"read_data","name":"datatable","version":"0.0.1","attrPaths":["datatable_selected","datatable_partition"],"attrs":[{"s":"alice-table"},{"s":"dt=20240101"}]}`)
	if nd.DatatableID() != "alice-table" {
		t.Fatalf("datatable id = %q", nd.DatatableID())
	}
	if nd.DatatablePartition() != "dt=20240101" {
		t.Fatalf("partition = %q", nd.DatatablePartition())
	}
	if nd.CompID() != "read_data/datatable" {
		t.Fatalf("comp id = %q", nd.CompID())
	}
	// snake_case twin keys from the case middleware are accepted
	nd2 := ParseNodeDef(`{"domain":"x","name":"y","attr_paths":["a"],"attrs":[{"s":"t1"}]}`)
	if len(nd2.AttrPaths) != 1 || nd2.DatatableID() != "t1" {
		t.Fatalf("snake attr paths not parsed: %+v", nd2)
	}
	hidden := ParseNodeDef(`{"attrs":[{"s":"x"},{"ss":["bob","alice"]}]}`)
	if hidden.HiddenPartyID() != "bob" {
		t.Fatalf("hidden party = %q", hidden.HiddenPartyID())
	}
}

func TestCoverCustomAttrs(t *testing.T) {
	nd := ParseNodeDef(`{"attrs":[{"custom_protobuf_cls":"x.Y","custom_value":{"a": 1}},{"i64":3}]}`)
	nd.CoverCustomAttrs()
	var a struct {
		S string `json:"s"`
	}
	if err := json.Unmarshal(nd.Attrs[0], &a); err != nil || a.S != `{"a":1}` {
		t.Fatalf("custom attr not covered: %s", nd.Attrs[0])
	}
	if string(nd.Attrs[1]) != `{"i64":3}` {
		t.Fatalf("plain attr changed: %s", nd.Attrs[1])
	}
}

func TestIsPad(t *testing.T) {
	n := GraphNodeInfo{CodeName: "read_data/datatable", NodeDef: json.RawMessage(`{"domain":"read_data","name":"datatable"}`)}
	if !n.IsPad() {
		t.Fatal("read_data should be pad")
	}
	n2 := GraphNodeInfo{CodeName: "data_prep/psi", NodeDef: json.RawMessage(`{"domain":"data_prep","name":"psi"}`)}
	if n2.IsPad() {
		t.Fatal("psi is not pad")
	}
	n3 := GraphNodeInfo{CodeName: "read_data/datatable", NodeDef: json.RawMessage(`{}`)}
	if !n3.IsPad() {
		t.Fatal("codeName fallback")
	}
}

func TestFindTopNodesAndBuilder(t *testing.T) {
	edges := []GraphEdge{
		{Source: "a", Target: "c"}, {Source: "b", Target: "c"}, {Source: "c", Target: "d"},
	}
	tops := FindTopNodes(edges, "d")
	if len(tops) != 2 || !tops["a"] || !tops["b"] {
		t.Fatalf("tops = %v", tops)
	}
	if tops := FindTopNodes(edges, "a"); len(tops) != 1 || !tops["a"] {
		t.Fatalf("root tops = %v", tops)
	}
	b := NewGraphBuilder([]GraphNodeInfo{{GraphNodeID: "c", Outputs: []string{"c-output-0"}}})
	if n, ok := b.NodeByInput("c-output-0"); !ok || n.GraphNodeID != "c" {
		t.Fatal("builder lookup failed")
	}
	if _, ok := b.NodeByInput("zz-output-0"); ok {
		t.Fatal("unknown input resolved")
	}
	parsed := ParseEdges(`[{"edge_id":"e1","source":"a","source_anchor":"a-output-0","target":"b","targetAnchor":"b-input-0"}]`)
	if len(parsed) != 1 || parsed[0].EdgeID != "e1" || parsed[0].SourceAnchor != "a-output-0" || parsed[0].TargetAnchor != "b-input-0" {
		t.Fatalf("parse edges: %+v", parsed)
	}
}

func TestIDsAndHelpers(t *testing.T) {
	if GenTaskID("abcd", "g-node-1") != "abcd-g-node-1" || GenTaskOutputID("abcd", "o") != "abcd-o" || GenExtendTaskID("t", "1") != "t--1" {
		t.Fatal("id helpers")
	}
	if UpstreamGraphNodeFromInput("g1-node-2-output-0") != "g1-node-2" {
		t.Fatalf("upstream = %s", UpstreamGraphNodeFromInput("g1-node-2-output-0"))
	}
	id := RandomLetters(4)
	if len(id) != 4 || strings.Trim(id, "abcdefghijklmnopqrstuvwxyz") != "" {
		t.Fatalf("random letters %q", id)
	}
	if OutputURI("ab-cd", "") != "ab_cd" || OutputURI("ab-cd", "20240101") != "ab_cd_20240101" {
		t.Fatal("output uri")
	}
	if PartitionRuleToDomainData("a,b") != "a;b" {
		t.Fatal("partition delimiter")
	}
}

func TestStatusMapping(t *testing.T) {
	cases := map[string]string{"Succeeded": TaskSucceed, "Failed": TaskFailed, "Running": TaskRunning, "Pending": TaskInitialized, "": TaskInitialized}
	for in, want := range cases {
		if got := TaskStatusFromKuscia(in); got != want {
			t.Errorf("task %q -> %q, want %q", in, got, want)
		}
	}
	if JobStatusFromKuscia("Succeeded") != JobSucceed || JobStatusFromKuscia("Pending") != JobRunning || JobStatusFromKuscia("Failed") != JobFailed {
		t.Fatal("job mapping")
	}
	if NormalizeTaskStatus("SUCCEEDED") != TaskSucceed || NormalizeTaskStatus("PENDING") != TaskInitialized || NormalizeTaskStatus("IDLE") != TaskStaging {
		t.Fatal("legacy task normalize")
	}
	if !IsJobFinished("SUCCEEDED") || IsJobFinished("PENDING") || !IsJobFinished(JobStopped) {
		t.Fatal("finished")
	}
	if ResultKindFromDatatable("table") != ResultFedTable || OutputTypeFromResultKind(ResultReport) != "report" || ResultKindFromDatatable("x") != "" {
		t.Fatal("result kinds")
	}
}

func TestTransitionLogs(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	logs := TransitionLogs(now, "j", "t", TaskInitialized, TaskSucceed, nil)
	if len(logs) != 2 || !strings.Contains(logs[0], "start ...") || !strings.Contains(logs[1], "succeed") {
		t.Fatalf("init->succeed logs %v", logs)
	}
	if !strings.HasPrefix(logs[0], "2026-01-02 03:04:05 INFO the jobId=j, taskId=t") {
		t.Fatalf("log format %q", logs[0])
	}
	logs = TransitionLogs(now, "j", "t", TaskRunning, TaskFailed, []string{"party bob failed msg: x", "boom"})
	if len(logs) != 2 || !strings.Contains(logs[0], "failed: party bob") {
		t.Fatalf("failed logs %v", logs)
	}
	if TransitionLogs(now, "j", "t", TaskRunning, TaskRunning, nil) != nil {
		t.Fatal("no transition must not log")
	}
	logs = TransitionLogs(now, "j", "t", TaskInitialized, TaskRunning, nil)
	if len(logs) != 1 {
		t.Fatalf("init->running %v", logs)
	}
	d := DistinctSpecifyLogs([]string{"a start x", "b start x", "c failed start x"}, "start x")
	if len(d) != 2 || d[0] != "a start x" {
		t.Fatalf("distinct %v", d)
	}
}

func TestTaskInputConfig(t *testing.T) {
	s := DefaultSettings()
	nd := ParseNodeDef(`{"domain":"data_prep","name":"psi","version":"1.0.0","attrPaths":["protocol"],"attrs":[{"s":"PROTOCOL_RR22"}]}`)
	cfg := &TaskInputConfig{
		DatasourceConfig:    map[string]DatasourceRef{"alice": {ID: "default-data-source"}},
		ClusterDesc:         BuildClusterDesc(s, []string{"alice", "bob"}),
		NodeEvalParam:       BuildEvalParam(nd, "ckjob-out"),
		InputIDs:            []string{"a", "b"},
		InputPartitionsSpec: []string{"", ""},
		OutputIDs:           []string{"job-out"},
		OutputURIs:          []string{"job_out"},
		TableAttrs:          []TableAttr{BuildTableAttr("a", []ColumnConfig{{ColName: "id", IsAssociateKey: true}, {ColName: "y", IsProtection: true}, {ColName: "x"}, {ColName: "g", IsGroupKey: true}})},
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(cfg.Marshal()), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["sf_input_partitions_spec"]; ok {
		t.Fatal("empty partition specs must be omitted")
	}
	eval := m["sf_node_eval_param"].(map[string]any)
	if eval["comp_id"] != "data_prep/psi:1.0.0" || eval["checkpoint_uri"] != "ckjob-out" {
		t.Fatalf("eval param %v", eval)
	}
	cluster := m["sf_cluster_desc"].(map[string]any)
	if len(cluster["devices"].([]any)) != 2 {
		t.Fatalf("devices %v", cluster)
	}
	attrs := m["table_attrs"].([]any)[0].(map[string]any)["column_attrs"].([]any)
	want := []string{"id", "label", "feature", "binning"}
	for i, a := range attrs {
		if a.(map[string]any)["col_type"] != want[i] {
			t.Fatalf("col attr %d = %v", i, a)
		}
	}
}

func TestParseColumnConfigs(t *testing.T) {
	cfgs := ParseColumnConfigs(`[{"colName":"a","col_type":"int","is_associate_key":true},{"col_name":"b","isProtection":true}]`)
	if len(cfgs) != 2 || !cfgs[0].IsAssociateKey || cfgs[0].ColType != "int" || cfgs[1].ColName != "b" || !cfgs[1].IsProtection {
		t.Fatalf("configs %+v", cfgs)
	}
}

func TestLoadSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "graph.yaml")
	yaml := `graph:
  max_parallelism: 3
  health_check: false
  devices:
    - name: spu
      config: '{"runtime_config":{"protocol":"ABY3"}}'
  hide: ["secretflow/x/y:1.0.0"]
  deploy_mode: MPC
  versions:
    secretpadImage: "9.9"
`
	if err := os.WriteFile(p, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings(p)
	if s.MaxParallelism != 3 || s.HealthCheck || len(s.Devices) != 1 || !s.IsHidden("secretflow/x/y:1.0.0") {
		t.Fatalf("settings %+v", s)
	}
	if v := s.VersionsForDeployMode(); v["secretpadImage"] != "9.9" {
		t.Fatalf("versions %v", v)
	}
	if d := BuildClusterDesc(s, []string{"a"}); d.Devices[0].Type != "spu" {
		t.Fatalf("device type default %+v", d)
	}
	if def := LoadSettings(filepath.Join(dir, "missing.yaml")); def.MaxParallelism != 1 || !def.HealthCheck {
		t.Fatal("defaults")
	}
}
