package datasync

import (
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

// Java P2pDataSyncProducerTemplate.filterVoteInviteDO: on the initiator a
// vote_invite change (a peer reply applied by the P2P sync) compensates
// vote_request.party_vote_info; non-initiators leave it alone.
func TestP2P_VoteInviteCompensatesInitiatorPartyInfo(t *testing.T) {
	db := newTestDB(t)
	db.Create(&model.VoteRequestDO{VoteID: "v1", Initiator: "instA", Type: "PROJECT_CREATE", Voters: "instB",
		PartyVoteInfo: `[{"partyId":"instB","action":"REVIEWING"},{"partyId":"instA","action":"APPROVED"}]`})
	inv := &model.VoteInviteDO{VoteID: "v1", Initiator: "instA", VoteParticipantID: "instB", Action: "REJECTED", Reason: "no"}

	// non-initiator: no compensation
	pb := NewP2PSync(db, nil, tables(DefaultP2PTables...), fakeIdentity{"instB"}, &fakePoster{}, "", 3, time.Hour, nil)
	pb.Filter(&ChangeEvent{Table: "vote_invite", Action: "update", Entity: inv, Route: RouteOf("vote_invite", inv)})
	var vr model.VoteRequestDO
	db.Where("vote_id = ?", "v1").First(&vr)
	if strings.Contains(vr.PartyVoteInfo, "REJECTED") {
		t.Fatalf("non-initiator compensated: %s", vr.PartyVoteInfo)
	}

	pa := NewP2PSync(db, nil, tables(DefaultP2PTables...), fakeIdentity{"instA"}, &fakePoster{}, "", 3, time.Hour, nil)
	if !pa.Filter(&ChangeEvent{Table: "vote_invite", Action: "update", Entity: inv, Route: RouteOf("vote_invite", inv)}) {
		t.Fatal("initiator must not re-sync an invite update")
	}
	db.Where("vote_id = ?", "v1").First(&vr)
	if !strings.Contains(vr.PartyVoteInfo, `{"partyId":"instB","action":"REJECTED","reason":"no"}`) ||
		!strings.Contains(vr.PartyVoteInfo, `{"partyId":"instA","action":"APPROVED"}`) {
		t.Fatalf("party info not compensated: %s", vr.PartyVoteInfo)
	}
}
