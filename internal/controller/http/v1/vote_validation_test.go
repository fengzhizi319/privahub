package v1

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service/votehandler"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// Java bean validation of MessageDetailRequest / VoteReplyRequest /
// CreateApprovalRequest (+ per-type voteConfig) / PullStatusRequest.
func TestVoteEndpoints_JavaValidation(t *testing.T) {
	user := &auth.UserContext{Name: "admin", OwnerID: "kuscia-system", OwnerType: auth.OwnerTypeCenter, PlatformType: auth.PlatformTypeCenter}
	r, _, _ := setupVoteEndpoints(t, "center", "kuscia-system", "", user)

	nodeRoute := map[string]interface{}{"srcNodeId": "alice", "desNodeId": "bob",
		"srcNodeAddr": "http://alice:1080", "desNodeAddr": "http://bob:1080"}
	tee := map[string]interface{}{"taskID": "t1", "resourceID": "r1", "jobID": "j1",
		"resourceType": "table", "projectID": "p1", "graphID": "g1"}
	without := func(m map[string]interface{}, k string) map[string]interface{} {
		out := map[string]interface{}{}
		for kk, v := range m {
			if kk != k {
				out[kk] = v
			}
		}
		return out
	}
	with := func(m map[string]interface{}, k string, v interface{}) map[string]interface{} {
		out := without(m, k)
		out[k] = v
		return out
	}

	cases := []struct {
		name, path string
		body       interface{}
		msg        string
	}{
		{"detail missing voteType", "message/detail", map[string]interface{}{"ownerId": "bob", "voteId": "v", "isInitiator": false}, "voteType"},
		{"detail bad voteType", "message/detail", map[string]interface{}{"ownerId": "bob", "voteId": "v", "isInitiator": false, "voteType": "X"}, "voteType"},
		{"detail node add not in api enum", "message/detail", map[string]interface{}{"ownerId": "bob", "voteId": "v", "isInitiator": false, "voteType": "PROJECT_NODE_ADD"}, "voteType"},
		{"detail lowercase voteType", "message/detail", map[string]interface{}{"ownerId": "bob", "voteId": "v", "isInitiator": false, "voteType": "node_route"}, "voteType"},
		{"detail missing isInitiator", "message/detail", map[string]interface{}{"ownerId": "bob", "voteId": "v", "voteType": "NODE_ROUTE"}, "isInitiator"},
		{"detail blank ownerId", "message/detail", map[string]interface{}{"ownerId": " ", "voteId": "v", "isInitiator": false, "voteType": "NODE_ROUTE"}, "ownerId"},
		{"reply reason too long", "message/reply", map[string]interface{}{"action": "REJECTED", "reason": strings.Repeat("理", 51), "voteId": "v", "voteParticipantId": "bob"}, "reason"},
		{"reply missing action", "message/reply", map[string]interface{}{"voteId": "v", "voteParticipantId": "bob"}, "action"},
		{"create bad voteType", "approval/create", map[string]interface{}{"initiatorId": "alice", "voteType": "PROJECT_NODE_ADD", "voteConfig": map[string]string{"projectId": "p"}}, "voteType"},
		{"create node route missing desNodeAddr", "approval/create", map[string]interface{}{"initiatorId": "alice", "voteType": "NODE_ROUTE", "voteConfig": without(nodeRoute, "desNodeAddr")}, "desNodeAddr"},
		{"create node route bad addr", "approval/create", map[string]interface{}{"initiatorId": "alice", "voteType": "NODE_ROUTE", "voteConfig": with(nodeRoute, "srcNodeAddr", "alice:1080")}, "http"},
		{"create tee missing taskID", "approval/create", map[string]interface{}{"initiatorId": "alice", "voteType": "TEE_DOWNLOAD", "voteConfig": without(tee, "taskID")}, "taskID"},
		{"create tee missing graphID", "approval/create", map[string]interface{}{"initiatorId": "alice", "voteType": "TEE_DOWNLOAD", "voteConfig": without(tee, "graphID")}, "graphID"},
		{"create project missing participants", "approval/create", map[string]interface{}{"initiatorId": "alice", "voteType": "PROJECT_CREATE", "voteConfig": map[string]interface{}{"projectId": "p"}}, "participants"},
		{"create archive missing projectId", "approval/create", map[string]interface{}{"initiatorId": "alice", "voteType": "PROJECT_ARCHIVE", "voteConfig": map[string]interface{}{}}, "projectId"},
		{"create missing voteConfig", "approval/create", map[string]interface{}{"initiatorId": "alice", "voteType": "PROJECT_ARCHIVE"}, "voteConfig"},
		{"pull missing resourceType", "approval/pull/status", map[string]string{"projectID": "p1", "jobID": "j1", "taskID": "t1", "resourceID": "r1"}, "resourceType"},
		{"pull bad resourceType", "approval/pull/status", map[string]string{"projectID": "p1", "jobID": "j1", "taskID": "t1", "resourceID": "r1", "resourceType": "report"}, "resourceType"},
	}
	for _, tc := range cases {
		resp := callVote(t, r, tc.path, tc.body)
		if resp.Status.Code != errcode.SystemValidationError.Code || !strings.Contains(resp.Status.Msg, tc.msg) {
			t.Errorf("%s: got %+v, want VALIDATION_ERROR mentioning %q", tc.name, resp.Status, tc.msg)
		}
	}
	// 50 characters is still accepted by validation (fails later: unknown vote).
	resp := callVote(t, r, "message/reply", map[string]interface{}{"action": "REJECTED", "reason": strings.Repeat("理", 50), "voteId": "v", "voteParticipantId": "bob"})
	if resp.Status.Code == errcode.SystemValidationError.Code {
		t.Errorf("50-char reason rejected: %+v", resp.Status)
	}
}

// approval/pull/status without taskID (result-list rows) resolves it from project_result.
func TestVoteEndpoints_PullStatusResolvesTaskID(t *testing.T) {
	user := &auth.UserContext{Name: "admin", OwnerID: "kuscia-system", OwnerType: auth.OwnerTypeCenter}
	r, db, _ := setupVoteEndpoints(t, "center", "kuscia-system", "", user)
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "j", Status: "SUCCEEDED", GraphID: "g1"})
	db.Create(&model.ProjectResultDO{ProjectID: "p1", Kind: "FedTable", NodeID: "alice", RefID: "r1", JobID: "j1", TaskID: "j1-t9"})
	resp := callVote(t, r, "approval/pull/status", map[string]string{"projectID": "p1", "jobID": "j1", "resourceID": "r1", "resourceType": "table"})
	var vo votehandler.PullStatusVO
	_ = json.Unmarshal(resp.Data, &vo)
	if resp.Status.Code != 0 || vo.TaskID != "j1-t9" || vo.GraphID != "g1" {
		t.Errorf("pull status = %+v %s", resp.Status, resp.Data)
	}
}
