package datasync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
)

func TestConsumer_ApplyFromPeer_Ownership(t *testing.T) {
	db := newTestDB(t)
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob", InstID: "instB"})
	db.Create(&model.ProjectDO{ProjectID: "pb", Name: "b", OwnerID: "instB"})
	db.Create(&model.ProjectDO{ProjectID: "pc", Name: "c", OwnerID: "instC"})
	db.Create(&model.VoteInviteDO{VoteID: "v-local", Initiator: "instA", VoteParticipantID: "bob", Type: "T", Description: "d"})
	db.Create(&model.VoteInviteDO{VoteID: "v-local", Initiator: "instA", VoteParticipantID: "carol", Type: "T", Description: "d"})
	c := NewConsumer(db, nil, tables(DefaultP2PTables...), "alice", nil).
		SetLocalIdentity(staticIdentity{inst: "instA", nodes: []string{"alice"}}, true)
	ctx := context.Background()
	apply := func(table, action, data string) error {
		_, err := c.ApplyFromPeer(ctx, "bob", &service.SyncData{TableName: table, Action: action, Data: json.RawMessage(data)})
		return err
	}
	cases := []struct {
		name, table, action, data string
		ok                        bool
	}{
		{"own project node", "project_node", "create", `{"project_id":"pb","node_id":"bob"}`, true},
		{"foreign project node", "project_node", "create", `{"project_id":"pc","node_id":"bob"}`, false},
		{"own vote request", "vote_request", "create", `{"vote_id":"v1","initiator":"instB","type":"t","voters":"[]","vote_counter":"a","executors":"[]","description":"d"}`, true},
		{"forged vote request", "vote_request", "create", `{"vote_id":"v2","initiator":"instA","type":"t","voters":"[]","vote_counter":"a","executors":"[]","description":"d"}`, false},
		{"invitee answers own invite", "vote_invite", "update", `{"vote_id":"v-local","initiator":"instA","vote_participant_id":"bob","type":"T","description":"d","action":"AGREE"}`, true},
		{"invitee answers someone else's invite", "vote_invite", "update", `{"vote_id":"v-local","initiator":"instA","vote_participant_id":"carol","type":"T","description":"d","action":"AGREE"}`, false},
		{"invitee cannot create invites of local votes", "vote_invite", "create", `{"vote_id":"v-new","initiator":"instA","vote_participant_id":"bob","type":"T","description":"d"}`, false},
		{"schedule of other owner", "project_schedule", "create", `{"schedule_id":"s1","project_id":"pc","graph_id":"g","owner":"instC"}`, false},
	}
	for _, tc := range cases {
		err := apply(tc.table, tc.action, tc.data)
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		if !tc.ok && !errors.Is(err, ErrNotOwner) {
			t.Errorf("%s: expected ErrNotOwner, got %v", tc.name, err)
		}
	}
	var carol model.VoteInviteDO
	db.Where("vote_id = ? AND vote_participant_id = ?", "v-local", "carol").First(&carol)
	if carol.Action == "AGREE" {
		t.Fatal("foreign invite answered")
	}
	if _, err := c.ApplyFromPeer(ctx, "", &service.SyncData{TableName: "project", Action: "create",
		Data: json.RawMessage(`{"project_id":"x","owner_id":"instB"}`)}); !errors.Is(err, ErrNotOwner) {
		t.Errorf("empty origin must be rejected: %v", err)
	}
}
