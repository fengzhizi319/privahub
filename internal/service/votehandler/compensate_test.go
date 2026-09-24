package votehandler

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

func TestCompensatePartyVoteInfo(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	infos := []PartyVoteInfo{{PartyID: "instB", Action: ActionReviewing}, {PartyID: "instA", Action: ActionApproved}}
	if err := db.Create(&model.VoteRequestDO{VoteID: "v1", Initiator: "instA", Type: VoteTypeProjectCreate,
		PartyVoteInfo: toJSON(infos)}).Error; err != nil {
		t.Fatal(err)
	}
	read := func() map[string]PartyVoteInfo {
		var vr model.VoteRequestDO
		db.Where("vote_id = ?", "v1").First(&vr)
		out := map[string]PartyVoteInfo{}
		for _, i := range parsePartyVoteInfos(vr.PartyVoteInfo) {
			out[i.PartyID] = i
		}
		return out
	}

	ok, err := CompensatePartyVoteInfo(ctx, db, &model.VoteInviteDO{VoteID: "v1", VoteParticipantID: "instB", Action: ActionApproved, Reason: "fine"})
	if err != nil || !ok {
		t.Fatalf("compensate = %v, %v", ok, err)
	}
	if got := read(); got["instB"].Action != ActionApproved || got["instB"].Reason != "fine" || got["instA"].Action != ActionApproved || len(got) != 2 {
		t.Fatalf("infos = %+v", got)
	}
	// unchanged -> no rewrite
	if ok, _ := CompensatePartyVoteInfo(ctx, db, &model.VoteInviteDO{VoteID: "v1", VoteParticipantID: "instB", Action: ActionApproved, Reason: "fine"}); ok {
		t.Error("unchanged entry rewritten")
	}
	// unknown party / vote: ignored like Java
	if ok, err := CompensatePartyVoteInfo(ctx, db, &model.VoteInviteDO{VoteID: "v1", VoteParticipantID: "instZ", Action: ActionRejected}); ok || err != nil {
		t.Errorf("unknown party = %v, %v", ok, err)
	}
	if ok, err := CompensatePartyVoteInfo(ctx, db, &model.VoteInviteDO{VoteID: "nope", VoteParticipantID: "instB", Action: ActionRejected}); ok || err != nil {
		t.Errorf("unknown vote = %v, %v", ok, err)
	}
	if len(read()) != 2 {
		t.Error("unknown party must not be added")
	}
}

func TestValidateVoteConfig(t *testing.T) {
	cases := []struct {
		voteType string
		cfg      *VoteConfig
		ok       bool
	}{
		{VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "a", DesNodeID: "b", SrcNodeAddr: "http://a", DesNodeAddr: "https://b"}, true},
		{VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "a", DesNodeID: "b", SrcNodeAddr: "a:1080", DesNodeAddr: "https://b"}, false},
		{VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "a", SrcNodeAddr: "http://a", DesNodeAddr: "https://b"}, false},
		{VoteTypeTeeDownload, &VoteConfig{TaskID: "t", ResourceID: "r", JobID: "j", ResourceType: "table", ProjectID: "p", GraphID: "g"}, true},
		{VoteTypeTeeDownload, &VoteConfig{ResourceID: "r", JobID: "j", ResourceType: "table", ProjectID: "p", GraphID: "g"}, false},
		{VoteTypeProjectCreate, &VoteConfig{ProjectID: "p", Participants: []string{"a"}}, true},
		{VoteTypeProjectCreate, &VoteConfig{ProjectID: "p"}, false},
		{VoteTypeProjectArchive, &VoteConfig{ProjectID: "p"}, true},
		{VoteTypeProjectArchive, &VoteConfig{ProjectID: "  "}, false},
		{VoteTypeProjectArchive, nil, false},
	}
	for i, c := range cases {
		if got := ValidateVoteConfig(c.voteType, c.cfg) == ""; got != c.ok {
			t.Errorf("case %d (%s): valid=%v, want %v (%q)", i, c.voteType, got, c.ok, ValidateVoteConfig(c.voteType, c.cfg))
		}
	}
	if ValidatePullStatus("p", "j", "r", "table") != "" || ValidatePullStatus("p", "j", "r", "") == "" || ValidatePullStatus("p", "j", "r", "report") == "" {
		t.Error("pull status resourceType enum")
	}
}
