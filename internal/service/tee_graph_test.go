package service

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func TestSM3_StandardVectors(t *testing.T) {
	h := sm3Sum([]byte("abc"))
	if got := hex.EncodeToString(h[:]); got != "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0" {
		t.Fatalf("sm3(abc)=%s", got)
	}
	h = sm3Sum([]byte(strings.Repeat("abcd", 16)))
	if got := hex.EncodeToString(h[:]); got != "debe9ff92275b8a138604889c18e5a4d6fdb70e5387e5765293dcba39c0c5732" {
		t.Fatalf("sm3(abcd*16)=%s", got)
	}
}

type fakeTeeSigner struct{}

func (fakeTeeSigner) InitiatorID() (string, error)      { return "INITIATOR", nil }
func (fakeTeeSigner) Sign(input string) (string, error) { return "sig(" + input + ")", nil }
func (fakeTeeSigner) Certs() ([]string, error)          { return []string{"client-crt", "ca-crt"}, nil }

func (f *graphFixture) makeTee() {
	f.t.Helper()
	f.mustCreate(&model.NodeDO{NodeID: "tee", Name: "TEE", ControlNodeID: "tee", Type: "tee"})
	if err := f.db.Model(&model.ProjectDO{}).Where("project_id = ?", "p1").
		Updates(map[string]any{"compute_mode": "TEE", "project_info": `{"teeDomainId":"tee"}`}).Error; err != nil {
		f.t.Fatal(err)
	}
	f.graph.SetTeeSigner(fakeTeeSigner{}).SetTeeConfig(TeeGraphConfig{CapsuleManager: "capsule-manager.#.svc"})
}

func TestStartGraph_TeeTrustedFlow(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.makeTee()

	vo, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3", "g1-node-4"}})
	if err != nil {
		t.Fatalf("start tee graph: %v", err)
	}
	if len(f.k.created) != 1 {
		t.Fatalf("create calls %d", len(f.k.created))
	}
	req := f.k.created[0]
	if req.Initiator != "tee" || len(req.Tasks) != 2 {
		t.Fatalf("req %+v", req)
	}
	psi, stats := req.Tasks[0], req.Tasks[1]
	if psi.AppImage != TeeAppImage || len(psi.Parties) != 1 || psi.Parties[0].DomainID != "tee" {
		t.Fatalf("psi task %+v", psi)
	}
	if len(stats.Dependencies) != 1 || stats.Dependencies[0] != psi.TaskID {
		t.Fatalf("deps %+v", stats.Dependencies)
	}
	cfg := decodeTaskInput(t, psi.InputConfig)
	eval := cfg["sf_node_eval_param"].(map[string]any)
	if eval["comp_id"] != "data_prep/psi:1.0.0" {
		t.Fatalf("comp id %v", eval["comp_id"])
	}
	ins := eval["inputs"].([]any)
	uri := ins[0].(map[string]any)["data_refs"].([]any)[0].(map[string]any)["uri"]
	if uri != "dm://input/?id=tee-alice-table" {
		t.Fatalf("input uri %v", uri)
	}
	out := vo.JobID + "-g1-node-3-output-0"
	if eval["output_uris"].([]any)[0] != "dm://output/?id="+out+"&&datasource_id=default-data-source&&uri="+out {
		t.Fatalf("output uris %v", eval["output_uris"])
	}
	tc := cfg["tee_task_config"].(map[string]any)
	if tc["capsule_manager_endpoint"] != "capsule-manager.tee.svc:80" || tc["scope"] != "p1" || tc["sign_algorithm"] != "RS256" ||
		tc["task_initiator_id"] != "INITIATOR" {
		t.Fatalf("tee task config %v", tc)
	}
	body := tc["task_body"].(string)
	if tc["signature"] != "sig(INITIATOR.p1."+body+")" {
		t.Fatalf("signature %v", tc["signature"])
	}
	raw, _ := base64.StdEncoding.DecodeString(body)
	var pipe map[string]any
	if err := json.Unmarshal(raw, &pipe); err != nil || pipe["domain"] != "data_prep" {
		t.Fatalf("task body %s", raw)
	}
	ds := cfg["sf_datasource_config"].(map[string]any)
	if len(ds) != 1 || ds["tee"] == nil {
		t.Fatalf("datasource %v", ds)
	}
	// tasks persisted with the TEE party.
	var task model.ProjectJobTaskDO
	f.db.Where("job_id = ? AND graph_node_id = ?", vo.JobID, "g1-node-3").First(&task)
	if !strings.Contains(task.Parties, "tee") {
		t.Fatalf("task parties %s", task.Parties)
	}
}

func TestRunGraphJob_TeeScheduled(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.makeTee()
	res, err := f.graph.RunGraphJob(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}},
		StartGraphOptions{Scheduled: true, ScheduleExpectStartDate: "20260924", JobID: "sch1"})
	if err != nil {
		t.Fatalf("scheduled tee render: %v", err)
	}
	if res.Request == nil || res.Request.Tasks[0].AppImage != TeeAppImage || len(f.k.created) != 0 {
		t.Fatalf("scheduled request %+v created=%d", res.Request, len(f.k.created))
	}
	var cnt int64
	f.db.Model(&model.ProjectJobDO{}).Where("job_id = ?", "sch1").Count(&cnt)
	if cnt != 0 {
		t.Fatalf("scheduled mode persisted job")
	}
}

func TestStartGraph_TeeSignFailure(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.makeTee()
	f.graph.SetTeeSigner(&FileTeeTaskSigner{CertDir: t.TempDir()})
	_, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}})
	pe, ok := AsPadError(err)
	if !ok || pe.Code != errcode.GraphDependentNodeNotRun || !strings.Contains(pe.Error(), "tee sign error") {
		t.Fatalf("err %v", err)
	}
	var cnt int64
	f.db.Model(&model.ProjectJobDO{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("failed job not rolled back")
	}
}

func TestFileTeeTaskSigner(t *testing.T) {
	dir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	pk8, _ := x509.MarshalPKCS8PrivateKey(key)
	_ = os.WriteFile(filepath.Join(dir, "ca.crt"), certPEM, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "client.crt"), certPEM, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "client.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk8}), 0o600)

	s := &FileTeeTaskSigner{CertDir: dir}
	id, err := s.InitiatorID()
	if err != nil || len(id) != 52 || strings.Contains(id, "=") {
		t.Fatalf("initiator id %q %v", id, err)
	}
	sig, err := s.Sign("hello")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(sig)
	sum := sha256.Sum256([]byte("hello"))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], raw); err != nil {
		t.Fatalf("verify: %v", err)
	}
	certs, err := s.Certs()
	if err != nil || len(certs) != 2 {
		t.Fatalf("certs %v %v", certs, err)
	}
}

// seedScqlGraph adds graph g2: alice-table, bob-table -> scql_analysis.
func (f *graphFixture) seedScqlGraph(initiator string) {
	f.t.Helper()
	edges := []graphjob.GraphEdge{
		{EdgeID: "e1", Source: "g2-node-1", SourceAnchor: "g2-node-1-output-0", Target: "g2-node-3", TargetAnchor: "g2-node-3-input-0"},
		{EdgeID: "e2", Source: "g2-node-2", SourceAnchor: "g2-node-2-output-0", Target: "g2-node-3", TargetAnchor: "g2-node-3-input-1"},
	}
	f.mustCreate(&model.ProjectGraphDO{ProjectID: "p1", GraphID: "g2", Name: "scql", Edges: graphjob.EdgesJSON(edges), OwnerID: "admin", NodeMaxIndex: 32})
	nodes := []graphjob.GraphNodeInfo{
		{CodeName: "read_data/datatable", GraphNodeID: "g2-node-1", Outputs: []string{"g2-node-1-output-0"},
			NodeDef: json.RawMessage(`{"domain":"read_data","name":"datatable","version":"0.0.1","attrPaths":["datatable_selected"],"attrs":[{"s":"alice-table"}]}`)},
		{CodeName: "read_data/datatable", GraphNodeID: "g2-node-2", Outputs: []string{"g2-node-2-output-0"},
			NodeDef: json.RawMessage(`{"domain":"read_data","name":"datatable","version":"0.0.1","attrPaths":["datatable_selected"],"attrs":[{"s":"bob-table"}]}`)},
		{CodeName: "stats/scql_analysis", GraphNodeID: "g2-node-3", Inputs: []string{"g2-node-1-output-0", "g2-node-2-output-0"}, Outputs: []string{"g2-node-3-output-0"},
			NodeDef: json.RawMessage(`{"domain":"stats","name":"scql_analysis","version":"1.0.0","attrPaths":["task_initiator","script_input"],"attrs":[{"ss":["` + initiator + `"]},{"s":"select count(*) from t"}]}`)},
	}
	for i := range nodes {
		f.mustCreate(nodeDOFromInfo("p1", "g2", &nodes[i]))
	}
}

func TestStartGraph_ScqlAnalysis(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.mustCreate(&model.NodeDO{NodeID: "carol", Name: "Carol", ControlNodeID: "carol", Type: "normal"})
	f.db.Model(&model.ProjectDatatableDO{}).Where("node_id = ? AND datatable_id = ?", "alice", "alice-table").Update("table_configs",
		`[{"colName":"id","colType":"str","isAssociateKey":true,"isProtection":true},{"colName":"g","colType":"str","isGroupKey":true,"isProtection":true},{"colName":"y","colType":"int","isProtection":true},{"colName":"x","colType":"float"}]`)
	f.seedScqlGraph("carol")

	vo, err := f.graph.StartGraph(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g2", Nodes: []string{"g2-node-1", "g2-node-2", "g2-node-3"}})
	if err != nil {
		t.Fatalf("start scql: %v", err)
	}
	task := f.k.created[0].Tasks[0]
	if task.AppImage != "scql-image" {
		t.Fatalf("image %s", task.AppImage)
	}
	parties := []string{}
	for _, p := range task.Parties {
		parties = append(parties, p.DomainID)
	}
	if strings.Join(parties, ",") != "alice,bob,carol" {
		t.Fatalf("parties %v", parties)
	}
	var cfg ScqlTaskInputConfig
	if err := json.Unmarshal([]byte(task.InputConfig), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectID != strings.ReplaceAll(task.TaskID, "-", "") || cfg.Initiator != "carol" || cfg.Query != "select count(*) from t" {
		t.Fatalf("cfg %+v", cfg)
	}
	if cfg.OutputIDs["carol"] != vo.JobID+"-g2-node-3-output-0" {
		t.Fatalf("output ids %v", cfg.OutputIDs)
	}
	at := cfg.Tables["alice"].Tbls[0]
	if at.TableOwner != "alice" || at.RefTable != "alice-table" || at.DBType != "csvdb" || len(at.Columns) != 4 {
		t.Fatalf("alice table %+v", at)
	}
	want := map[string]string{}
	for _, c := range cfg.Ccls["alice"].ColumnControlList {
		want[c.Col.ColumnName+"/"+c.PartyCode] = c.Constraint
		if c.Col.TableName != at.TableName {
			t.Fatalf("ccl table name %s != %s", c.Col.TableName, at.TableName)
		}
	}
	checks := map[string]string{
		"id/alice": "PLAINTEXT", "id/bob": "PLAINTEXT_AFTER_JOIN", "g/carol": "PLAINTEXT_AFTER_GROUP_BY",
		"y/bob": "PLAINTEXT_AFTER_AGGREGATE", "x/carol": "PLAINTEXT",
	}
	for k, v := range checks {
		if want[k] != v {
			t.Fatalf("ccl %s = %q want %q (all %v)", k, want[k], v, want)
		}
	}
	if len(cfg.Ccls["bob"].ColumnControlList) != 3 { // 1 column x 3 parties
		t.Fatalf("bob ccl %+v", cfg.Ccls["bob"])
	}
}

func TestScqlHelpers(t *testing.T) {
	if got := scqlTableName("abcd-node-1-output-0", "alice-1"); got != "alice_1_node_1_output_0" {
		t.Fatalf("table name %s", got)
	}
	if got := scqlTableName("tbl", "alice-1"); got != "alice_1_tbl" {
		t.Fatalf("table name %s", got)
	}
	if scqlDBType("mysql-ds") != "mysql" || scqlDBType("odps-x") != "odps" || scqlDBType("default-data-source") != "csvdb" {
		t.Fatal("db type")
	}
}

func (f *graphFixture) seedPartitionedRead(rule string) {
	f.t.Helper()
	f.db.Unscoped().Where("project_id = ? AND graph_id = ? AND graph_node_id = ?", "p1", "g1", "g1-node-1").Delete(&model.ProjectGraphNodeDO{})
	n := graphjob.GraphNodeInfo{CodeName: "read_data/datatable", GraphNodeID: "g1-node-1", Outputs: []string{"g1-node-1-output-0"},
		NodeDef: json.RawMessage(`{"domain":"read_data","name":"datatable","version":"0.0.1","attrPaths":["datatable_selected","datatable_partition"],"attrs":[{"s":"alice-table"},{"s":"` + rule + `"}]}`)}
	f.mustCreate(nodeDOFromInfo("p1", "g1", &n))
	dd := f.k.domainData["alice/alice-table"]
	dd.Data.Partition = &kuscia.Partition{Type: kuscia.PartitionTypeOdps, Fields: []kuscia.DataColumn{{Name: "dt", Type: "str"}}}
}

func TestStartGraph_OdpsPartitionColumns(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	f.seedPartitionedRead("dt=${yyyymmdd}")
	res, err := f.graph.RunGraphJob(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}},
		StartGraphOptions{Scheduled: true, ScheduleExpectStartDate: "20260924", JobID: "odp1"})
	if err != nil {
		t.Fatalf("partition render: %v", err)
	}
	cfg := decodeTaskInput(t, res.Request.Tasks[0].InputConfig)
	specs := cfg["sf_input_partitions_spec"].([]any)
	if specs[0] != "dt='20260924'" || specs[1] != "" {
		t.Fatalf("specs %v", specs)
	}

	g := newGraphFixture(t)
	g.seedPSIProject()
	g.seedPartitionedRead("ds=${yyyymmdd}") // ds is not a partition column
	_, err = g.graph.RunGraphJob(adminCtx(), &StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}},
		StartGraphOptions{Scheduled: true, ScheduleExpectStartDate: "20260924", JobID: "odp2"})
	if pe, ok := AsPadError(err); !ok || pe.Code != errcode.SystemValidationError {
		t.Fatalf("want partition column validation error, got %v", err)
	}
}

func withGlobalEnv(t *testing.T, env *EnvService) {
	t.Helper()
	globalEnvMu.RLock()
	prev := globalEnv
	globalEnvMu.RUnlock()
	SetGlobalEnv(env)
	t.Cleanup(func() { SetGlobalEnv(prev) })
}

func TestVerifyNodeAndRouteHealthy_Autonomy(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	env := NewEnvService("AUTONOMY", "alice", "inst-a", nil)
	withGlobalEnv(t, env)
	f.db.Model(&model.NodeDO{}).Where("node_id = ?", "alice").Update("inst_id", "inst-a")
	f.db.Model(&model.NodeDO{}).Where("node_id = ?", "bob").Update("inst_id", "inst-b")
	ctx := WithOperator(context.Background(), Operator{OwnerID: "inst-a", OwnerType: "P2P"})

	// single party: always fine.
	if err := f.graph.verifyNodeAndRouteHealthy(ctx, "p1", map[string]bool{"alice": true}); err != nil {
		t.Fatal(err)
	}
	// no approval config -> PROJECT_NOT_EXISTS.
	err := f.graph.verifyNodeAndRouteHealthy(ctx, "p1", map[string]bool{"alice": true, "bob": true})
	if pe, ok := AsPadError(err); !ok || pe.Code != errcode.ProjectNotExists {
		t.Fatalf("want project not exists, got %v", err)
	}
	f.mustCreate(&model.ProjectApprovalConfigDO{VoteID: "v1", Initiator: "alice", Type: "PROJECT_CREATE", ProjectID: "p1",
		ParticipantNodeInfo: `[{"initiatorNodeId":"alice","invitees":[{"inviteeId":"bob"}]}]`})
	// no route bob->alice at all.
	err = f.graph.verifyNodeAndRouteHealthy(ctx, "p1", map[string]bool{"alice": true, "bob": true})
	if pe, ok := AsPadError(err); !ok || pe.Code != errcode.GraphNodeRouteNotExists {
		t.Fatalf("want route error, got %v", err)
	}
	f.mustCreate(&model.NodeRouteDO{RouteID: "r1", SrcNodeID: "bob", DstNodeID: "alice"})
	if err := f.graph.verifyNodeAndRouteHealthy(ctx, "p1", map[string]bool{"alice": true, "bob": true}); err != nil {
		t.Fatalf("ready route: %v", err)
	}
	// route not ready and bob is invited by alice -> error.
	f.k.routeNotReady["bob->alice"] = true
	err = f.graph.verifyNodeAndRouteHealthy(ctx, "p1", map[string]bool{"alice": true, "bob": true})
	if pe, ok := AsPadError(err); !ok || pe.Code != errcode.GraphNodeRouteNotExists || !strings.Contains(pe.Error(), "bob-> alice") {
		t.Fatalf("want unavailable route error, got %v", err)
	}
}

func TestAutonomyInitiatorAndTargetNode(t *testing.T) {
	f := newGraphFixture(t)
	f.seedPSIProject()
	env := NewEnvService("AUTONOMY", "bob", "inst-b", nil)
	withGlobalEnv(t, env)
	if got := f.graph.findLocalNodeID(context.Background(), "p1", []string{"alice", "bob"}); got != "bob" {
		t.Fatalf("local node %s", got)
	}
	if got := f.graph.findLocalNodeID(context.Background(), "p1", []string{"alice"}); got != "bob" {
		t.Fatalf("local project node %s", got)
	}
	// alice belongs to another institution: read it through a project node
	// that is not a task party.
	f.mustCreate(&model.NodeDO{NodeID: "carol", Name: "Carol", ControlNodeID: "carol"})
	f.mustCreate(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "carol"})
	if got := autonomyTargetNode(context.Background(), f.db, "p1", "alice", []string{"alice", "bob"}); got != "carol" {
		t.Fatalf("target %s", got)
	}
	if got := autonomyTargetNode(context.Background(), f.db, "p1", "bob", []string{"alice", "bob"}); got != "bob" {
		t.Fatalf("local target %s", got)
	}

	vo, err := f.graph.StartGraph(WithOperator(context.Background(), Operator{OwnerID: "admin", OwnerType: "P2P"}),
		&StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-3"}})
	if err == nil {
		t.Fatalf("expected approval config error in autonomy mode, job %s", vo.JobID)
	}
	f.mustCreate(&model.ProjectApprovalConfigDO{VoteID: "v1", Initiator: "alice", Type: "PROJECT_CREATE", ProjectID: "p1",
		ParticipantNodeInfo: `[{"initiatorNodeId":"alice","invitees":[{"inviteeId":"bob"}]}]`})
	f.mustCreate(&model.NodeRouteDO{RouteID: "r1", SrcNodeID: "alice", DstNodeID: "bob"})
	f.db.Model(&model.NodeDO{}).Where("node_id = ?", "bob").Update("inst_id", "admin")
	// P2P: read_data grants are checked on the local node.
	f.k.addDomainData("bob", "alice-table", "table", nil, nil)
	if _, err := f.graph.StartGraph(WithOperator(context.Background(), Operator{OwnerID: "admin", OwnerType: "P2P"}),
		&StartGraphRequest{ProjectID: "p1", GraphID: "g1", Nodes: []string{"g1-node-1", "g1-node-2", "g1-node-3"}}); err != nil {
		t.Fatalf("autonomy start: %v", err)
	}
	if f.k.created[0].Initiator != "bob" {
		t.Fatalf("autonomy initiator %s", f.k.created[0].Initiator)
	}
}

func TestRebuildTeeResultID(t *testing.T) {
	n, r := RebuildTeeResultID("alice", "alice-abcd-g1-node-3-output-0", "tee")
	if n != "tee" || r != "abcd-g1-node-3-output-0" {
		t.Fatalf("%s %s", n, r)
	}
}
