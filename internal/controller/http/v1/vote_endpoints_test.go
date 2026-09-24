package v1

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/internal/service/votehandler"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type voteRespBody struct {
	Status struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	} `json:"status"`
	Data json.RawMessage `json:"data"`
}

func setupVoteEndpoints(t *testing.T, mode, nodeID, instID string, user *auth.UserContext) (*gin.Engine, *gorm.DB, *votehandler.Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vote.db")+"?_pragma=busy_timeout(5000)"),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.VoteRequestDO{}, &model.VoteInviteDO{}, &model.TeeDownloadApprovalConfigDO{},
		&model.NodeRouteApprovalConfigDO{}, &model.ProjectApprovalConfigDO{}, &model.NodeDO{}, &model.InstDO{},
		&model.NodeRouteDO{}, &model.ProjectDO{}, &model.ProjectNodeDO{}, &model.ProjectInstDO{},
		&model.ProjectJobDO{}, &model.ProjectJobTaskDO{}, &model.ProjectGraphDO{}, &model.ProjectDatatableDO{},
		&model.SysUserPermissionRelDO{}, &model.EdgeDataSyncLogDO{}, &model.ProjectResultDO{}); err != nil {
		t.Fatal(err)
	}
	env := service.NewEnvService(mode, nodeID, instID, nil)
	if env.IsCenter() {
		env.SetEmbeddedNodes([]string{"alice", "bob"})
	}
	middleware.SetDataResourceAuth(service.NewDataResourceAuth(db, env))
	t.Cleanup(func() { middleware.SetDataResourceAuth(nil) })
	// offline mode (no Kuscia): signing is skipped
	d := &votehandler.Deps{DB: db, Env: env}
	svc := votehandler.NewService(d)
	p2p := votehandler.NewP2PService(d)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if user != nil {
			c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), user))
			c.Set(middleware.CtxUserContext, user)
		}
	})
	mh, ah, ph := NewMessageHandler(svc), NewApprovalHandler(svc), NewP2PHandler(db, p2p, zap.NewNop())
	api := r.Group("/api/v1alpha1")
	api.POST("/message/list", mh.List)
	api.POST("/message/detail", mh.Detail)
	api.POST("/message/reply", mh.Reply)
	api.POST("/message/pending", mh.Pending)
	api.POST("/approval/create", ah.Create)
	api.POST("/approval/pull/status", ah.PullStatus)
	api.POST("/p2p/project/create", ph.ProjectCreate)
	api.POST("/p2p/project/list", ph.ProjectList)
	api.POST("/p2p/project/update", ph.ProjectUpdate)
	api.POST("/p2p/project/archive", ph.ProjectArchive)
	api.POST("/p2p/project/participants", ph.ProjectParticipants)
	api.POST("/p2p/node/create", ph.NodeCreate)
	api.POST("/p2p/node/delete", ph.NodeDelete)
	return r, db, svc
}

func callVote(t *testing.T, r *gin.Engine, path string, body interface{}) voteRespBody {
	t.Helper()
	w := postJSON(r, "/api/v1alpha1/"+path, body)
	if w.Code != http.StatusOK {
		t.Fatalf("%s: http %d %s", path, w.Code, w.Body.String())
	}
	var resp voteRespBody
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return resp
}

func TestVoteEndpoints_CenterNodeRouteFlow(t *testing.T) {
	user := &auth.UserContext{Name: "admin", OwnerID: "kuscia-system", OwnerType: auth.OwnerTypeCenter, PlatformType: auth.PlatformTypeCenter}
	r, db, svc := setupVoteEndpoints(t, "center", "kuscia-system", "", user)
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob"})

	// validation: old snake_case contract is rejected
	if resp := callVote(t, r, "approval/create", map[string]interface{}{"initiator": "alice", "voters": []string{"bob"}, "type": "X"}); resp.Status.Code != errcode.SystemValidationError.Code {
		t.Errorf("old contract accepted: %+v", resp.Status)
	}
	resp := callVote(t, r, "approval/create", map[string]interface{}{
		"initiatorId": "alice", "voteType": "NODE_ROUTE",
		"voteConfig": map[string]interface{}{"srcNodeId": "alice", "desNodeId": "bob",
			"srcNodeAddr": "http://alice:1080", "desNodeAddr": "http://bob:1080", "isSingle": true},
	})
	if resp.Status.Code != 0 {
		t.Fatalf("create: %+v", resp.Status)
	}
	var created struct {
		VoteID string `json:"voteId"`
	}
	_ = json.Unmarshal(resp.Data, &created)

	resp = callVote(t, r, "message/pending", map[string]string{"ownerId": "bob"})
	if string(resp.Data) != "1" {
		t.Errorf("pending = %s", resp.Data)
	}
	resp = callVote(t, r, "message/list", map[string]interface{}{"page": 1, "size": 10, "isInitiator": false, "ownerId": "bob"})
	var list struct {
		Messages []map[string]interface{} `json:"messages"`
		Total    int                      `json:"total"`
	}
	_ = json.Unmarshal(resp.Data, &list)
	if list.Total != 1 || list.Messages[0]["voteID"] != created.VoteID || list.Messages[0]["status"] != "REVIEWING" {
		t.Fatalf("list = %s", resp.Data)
	}
	itm := list.Messages[0]["initiatingTypeMessage"].(map[string]interface{})
	if itm["initiatorNodeName"] != "Alice" || itm["participantID"] != "bob" {
		t.Errorf("initiatingTypeMessage = %v", itm)
	}
	resp = callVote(t, r, "message/detail", map[string]interface{}{"ownerId": "bob", "voteId": created.VoteID, "isInitiator": false, "voteType": "NODE_ROUTE"})
	var detail map[string]interface{}
	_ = json.Unmarshal(resp.Data, &detail)
	for k, want := range map[string]interface{}{"initiatorNodeName": "Alice", "nodeName": "Bob", "nodeID": "bob",
		"url": "http://alice:1080", "isSingle": true, "type": "NODE_ROUTE", "status": "REVIEWING"} {
		if detail[k] != want {
			t.Errorf("detail[%s] = %v, want %v", k, detail[k], want)
		}
	}
	if resp := callVote(t, r, "message/reply", map[string]string{"action": "AGREE", "voteId": created.VoteID, "voteParticipantId": "bob"}); resp.Status.Code != errcode.SystemValidationError.Code {
		t.Errorf("AGREE accepted: %+v", resp.Status)
	}
	if resp := callVote(t, r, "message/reply", map[string]string{"action": "APPROVED", "reason": "ok", "voteId": created.VoteID, "voteParticipantId": "bob"}); resp.Status.Code != 0 {
		t.Fatalf("reply: %+v", resp.Status)
	}
	if resp := callVote(t, r, "message/reply", map[string]string{"action": "APPROVED", "voteId": "nope", "voteParticipantId": "bob"}); resp.Status.Code != errcode.VoteNotExists.Code {
		t.Errorf("unknown vote: %+v", resp.Status)
	}
	var inv model.VoteInviteDO
	db.Where("vote_id = ? AND vote_participant_id = ?", created.VoteID, "bob").First(&inv)
	if inv.Action != "APPROVED" || inv.Reason != "ok" {
		t.Errorf("invite = %+v", inv)
	}
	mon := votehandler.NewMonitor(svc, votehandler.MonitorOptions{})
	mon.SyncInvites(t.Context())
	var vr model.VoteRequestDO
	db.Where("vote_id = ?", created.VoteID).First(&vr)
	if vr.Status != votehandler.StatusApproved {
		t.Errorf("vote status = %d", vr.Status)
	}
	resp = callVote(t, r, "message/list", map[string]interface{}{"isInitiator": true, "ownerId": "alice"})
	_ = json.Unmarshal(resp.Data, &list)
	if list.Messages[0]["status"] != "APPROVED" {
		t.Errorf("initiator list = %s", resp.Data)
	}
}

func TestVoteEndpoints_DataResourceDenied(t *testing.T) {
	user := &auth.UserContext{Name: "alice", OwnerID: "alice", OwnerType: auth.OwnerTypeEdge, PlatformType: auth.PlatformTypeEdge}
	r, _, _ := setupVoteEndpoints(t, "edge", "alice", "", user)
	for path, body := range map[string]interface{}{
		"message/list":    map[string]interface{}{"ownerId": "bob"},
		"message/pending": map[string]interface{}{"ownerId": "bob"},
		"message/detail":  map[string]interface{}{"ownerId": "bob", "voteId": "v", "isInitiator": false, "voteType": "NODE_ROUTE"},
		"message/reply":   map[string]interface{}{"action": "APPROVED", "voteId": "v", "voteParticipantId": "bob"},
		"approval/create": map[string]interface{}{"initiatorId": "bob", "voteType": "NODE_ROUTE", "voteConfig": map[string]string{
			"srcNodeId": "bob", "desNodeId": "alice", "srcNodeAddr": "http://bob:1080", "desNodeAddr": "http://alice:1080"}},
	} {
		if resp := callVote(t, r, path, body); resp.Status.Code != errcode.AuthFailed.Code {
			t.Errorf("%s: %+v", path, resp.Status)
		}
	}
	if resp := callVote(t, r, "message/list", map[string]interface{}{}); resp.Status.Code != errcode.SystemValidationError.Code {
		t.Errorf("missing ownerId: %+v", resp.Status)
	}
}

func TestVoteEndpoints_PullStatus(t *testing.T) {
	user := &auth.UserContext{Name: "admin", OwnerID: "kuscia-system", OwnerType: auth.OwnerTypeCenter}
	r, db, _ := setupVoteEndpoints(t, "center", "kuscia-system", "", user)
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice"})
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "j", Status: "SUCCEEDED", GraphID: "g1"})
	resp := callVote(t, r, "approval/pull/status", map[string]string{"projectID": "p1", "jobID": "j1", "taskID": "t1", "resourceID": "r1", "resourceType": "table"})
	var vo votehandler.PullStatusVO
	_ = json.Unmarshal(resp.Data, &vo)
	if resp.Status.Code != 0 || vo.GraphID != "g1" || vo.TaskID != "t1" || len(vo.Parties) != 1 || vo.Parties[0].Status != "NOT_INITIATED" || vo.Parties[0].NodeName != "Alice" {
		t.Errorf("pull status = %+v %s", resp.Status, resp.Data)
	}
	if resp := callVote(t, r, "approval/pull/status", map[string]string{"projectID": "p1", "jobID": "nope", "resourceID": "r1", "resourceType": "table"}); resp.Status.Code != errcode.ProjectJobNotExists.Code {
		t.Errorf("unknown job: %+v", resp.Status)
	}
}

func TestVoteEndpoints_P2PProjects(t *testing.T) {
	user := &auth.UserContext{Name: "instA", OwnerID: "instA", OwnerType: auth.OwnerTypeP2P, PlatformType: auth.PlatformTypeAutonomy}
	r, db, _ := setupVoteEndpoints(t, "autonomy", "a1", "instA", user)
	db.Create(&model.InstDO{InstID: "instA", Name: "Inst A"})
	db.Create(&model.InstDO{InstID: "instB", Name: "Inst B"})
	db.Create(&model.NodeDO{NodeID: "a1", Name: "A1", ControlNodeID: "a1", InstID: "instA"})
	db.Create(&model.NodeDO{NodeID: "b1", Name: "B1", ControlNodeID: "b1", InstID: "instB"})

	resp := callVote(t, r, "p2p/project/create", map[string]string{"name": "joint", "description": "d", "computeMode": "MPC", "computeFunc": "ALL"})
	var created struct {
		ProjectID string `json:"projectId"`
	}
	_ = json.Unmarshal(resp.Data, &created)
	if resp.Status.Code != 0 || created.ProjectID == "" {
		t.Fatalf("create: %+v %s", resp.Status, resp.Data)
	}
	resp = callVote(t, r, "approval/create", map[string]interface{}{
		"initiatorId": "instA", "voteType": "PROJECT_CREATE",
		"voteConfig": map[string]interface{}{"projectId": created.ProjectID, "participants": []string{"instA", "instB"},
			"participantNodeInstVOS": []map[string]interface{}{{"initiatorNodeId": "a1", "invitees": []map[string]string{{"inviteeId": "b1"}}}}},
	})
	if resp.Status.Code != 0 {
		t.Fatalf("approval: %+v", resp.Status)
	}
	resp = callVote(t, r, "p2p/project/list", map[string]string{})
	var list []map[string]interface{}
	_ = json.Unmarshal(resp.Data, &list)
	if len(list) != 1 || list[0]["projectId"] != created.ProjectID || list[0]["status"] != "REVIEWING" || list[0]["voteId"] == "" ||
		list[0]["initiatorName"] != "Inst A" {
		t.Fatalf("list = %s", resp.Data)
	}
	voteID := list[0]["voteId"].(string)
	resp = callVote(t, r, "p2p/project/participants", map[string]string{"voteId": voteID})
	var parts map[string]interface{}
	_ = json.Unmarshal(resp.Data, &parts)
	if parts["projectName"] != "joint" || len(parts["participantNodeInstVOS"].([]interface{})) != 1 || len(parts["partyVoteStatuses"].([]interface{})) != 2 {
		t.Errorf("participants = %s", resp.Data)
	}
	resp = callVote(t, r, "message/detail", map[string]interface{}{"ownerId": "instA", "voteId": voteID, "isInitiator": true, "voteType": "PROJECT_CREATE"})
	var detail map[string]interface{}
	_ = json.Unmarshal(resp.Data, &detail)
	if resp.Status.Code != 0 || detail["projectName"] != "joint" || detail["initiatorName"] != "Inst A" {
		t.Errorf("detail = %+v %s", resp.Status, resp.Data)
	}
	// @DataResource(PROJECT_ID): in AUTONOMY the owner inst must be a project member
	if resp := callVote(t, r, "p2p/project/update", map[string]string{"projectId": created.ProjectID, "name": "x"}); resp.Status.Code != errcode.AuthFailed.Code {
		t.Errorf("update by non-member: %+v", resp.Status)
	}
	db.Create(&model.ProjectInstDO{ProjectID: created.ProjectID, InstID: "instA"})
	if resp := callVote(t, r, "p2p/project/update", map[string]string{"projectId": created.ProjectID, "name": "renamed"}); resp.Status.Code != 0 {
		t.Errorf("update: %+v", resp.Status)
	}
	if resp := callVote(t, r, "p2p/project/archive", map[string]string{"projectId": created.ProjectID}); resp.Status.Code != 0 {
		t.Errorf("archive: %+v", resp.Status)
	}
	var p model.ProjectDO
	db.Where("project_id = ?", created.ProjectID).First(&p)
	if p.Name != "renamed" || p.Status != 2 {
		t.Errorf("project = %+v", p)
	}
	if resp := callVote(t, r, "p2p/project/participants", map[string]string{}); resp.Status.Code != errcode.SystemValidationError.Code {
		t.Errorf("missing voteId: %+v", resp.Status)
	}
}

func TestVoteEndpoints_P2PNodes(t *testing.T) {
	user := &auth.UserContext{Name: "instA", OwnerID: "instA", OwnerType: auth.OwnerTypeP2P, PlatformType: auth.PlatformTypeAutonomy}
	r, db, _ := setupVoteEndpoints(t, "autonomy", "a1", "instA", user)
	db.Create(&model.NodeDO{NodeID: "a1", Name: "A1", ControlNodeID: "a1", InstID: "instA"})
	resp := callVote(t, r, "p2p/node/create", map[string]interface{}{"name": "peer", "certText": "CERT", "dstNodeId": "b1",
		"srcNodeId": "a1", "srcNetAddress": "http://a1:1080", "dstNetAddress": "http://b1:1080", "dstInstId": "instB", "dstInstName": "Inst B", "mode": 0})
	if resp.Status.Code != 0 || string(resp.Data) != `"b1"` {
		t.Fatalf("create node: %+v %s", resp.Status, resp.Data)
	}
	if resp := callVote(t, r, "p2p/node/create", map[string]interface{}{"dstNodeId": "b2", "srcNodeId": "a1", "dstNetAddress": "b2"}); resp.Status.Code != errcode.SystemValidationError.Code {
		t.Errorf("invalid request: %+v", resp.Status)
	}
	db.Create(&model.NodeRouteDO{RouteID: "b1__a1", SrcNodeID: "b1", DstNodeID: "a1"})
	if resp := callVote(t, r, "p2p/node/delete", map[string]string{"routerId": "b1__a1"}); resp.Status.Code != 0 {
		t.Fatalf("delete: %+v", resp.Status)
	}
	if resp := callVote(t, r, "p2p/node/delete", map[string]string{"routerId": "missing"}); resp.Status.Code != errcode.NodeRouteNotExistError.Code {
		t.Errorf("missing route: %+v", resp.Status)
	}
	if resp := callVote(t, r, "p2p/node/delete", map[string]string{"node_id": "b1"}); resp.Status.Code != errcode.SystemValidationError.Code {
		t.Errorf("old contract: %+v", resp.Status)
	}
}
