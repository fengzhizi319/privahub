package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupBugfix4TestDB creates an in-memory SQLite database for bug fix 32-36 tests.
func setupBugfix4TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.VoteRequestDO{},
		&model.VoteInviteDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// --- Bug 34: VoteService.CreateVote atomicity ---

func TestVoteService_CreateVote_Atomic(t *testing.T) {
	db := setupBugfix4TestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	vo, err := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator: "alice",
		Type:      "PROJECT_CREATE",
		Voters:    []string{"bob", "charlie"},
		Executors: []string{"bob"},
	})
	if err != nil {
		t.Fatalf("CreateVote failed: %v", err)
	}
	if vo.VoteID == "" {
		t.Fatal("expected non-empty vote ID")
	}

	// Verify vote was persisted
	var vote model.VoteRequestDO
	if err := db.Where("vote_id = ?", vo.VoteID).First(&vote).Error; err != nil {
		t.Fatalf("vote not found in DB: %v", err)
	}

	// Verify all invites were persisted atomically
	var invites []model.VoteInviteDO
	db.Where("vote_id = ?", vo.VoteID).Find(&invites)
	if len(invites) != 2 {
		t.Fatalf("expected 2 invites, got %d", len(invites))
	}
	for _, inv := range invites {
		if inv.Action != "REVIEWING" {
			t.Errorf("expected invite action REVIEWING, got %q", inv.Action)
		}
	}
}

// --- Bug 35: VoteService.ReplyVote atomicity ---

func TestVoteService_ReplyVote_Atomic(t *testing.T) {
	db := setupBugfix4TestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	// Create a vote with 2 voters, threshold = 2
	vo, err := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator:         "alice",
		Type:              "DATA_GRANT",
		Voters:            []string{"bob", "charlie"},
		ApprovedThreshold: 2,
	})
	if err != nil {
		t.Fatalf("CreateVote failed: %v", err)
	}

	// First reply: AGREE (1/2 — should not change vote status)
	if err := svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            vo.VoteID,
		VoteParticipantID: "bob",
		Action:            "AGREE",
	}); err != nil {
		t.Fatalf("ReplyVote(bob) failed: %v", err)
	}

	var vote model.VoteRequestDO
	db.Where("vote_id = ?", vo.VoteID).First(&vote)
	if vote.Status != 0 {
		t.Fatalf("expected vote status PENDING(0) after 1/2 agrees, got %d", vote.Status)
	}

	// Second reply: AGREE (2/2 — should approve)
	if err := svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            vo.VoteID,
		VoteParticipantID: "charlie",
		Action:            "AGREE",
	}); err != nil {
		t.Fatalf("ReplyVote(charlie) failed: %v", err)
	}

	db.Where("vote_id = ?", vo.VoteID).First(&vote)
	if vote.Status != 1 {
		t.Fatalf("expected vote status APPROVED(1) after 2/2 agrees, got %d", vote.Status)
	}
}

func TestVoteService_ReplyVote_Reject(t *testing.T) {
	db := setupBugfix4TestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	// Create a vote with 3 voters, threshold = 2
	vo, err := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator:         "alice",
		Type:              "PROJECT_DELETE",
		Voters:            []string{"bob", "charlie", "dave"},
		ApprovedThreshold: 2,
	})
	if err != nil {
		t.Fatalf("CreateVote failed: %v", err)
	}

	// Two rejections: with 3 voters and threshold 2, rejection happens when
	// rejectCount > len(invites) - threshold = 3 - 2 = 1, i.e. 2 rejects.
	_ = svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            vo.VoteID,
		VoteParticipantID: "bob",
		Action:            "REJECT",
	})
	_ = svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            vo.VoteID,
		VoteParticipantID: "charlie",
		Action:            "REJECT",
	})

	var vote model.VoteRequestDO
	db.Where("vote_id = ?", vo.VoteID).First(&vote)
	if vote.Status != 2 {
		t.Fatalf("expected vote status REJECTED(2) after 2 rejects, got %d", vote.Status)
	}
}

func TestVoteService_ReplyVote_NotFound(t *testing.T) {
	db := setupBugfix4TestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	err := svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            "nonexistent",
		VoteParticipantID: "ghost",
		Action:            "AGREE",
	})
	if err != ErrVoteNotFound {
		t.Fatalf("expected ErrVoteNotFound, got %v", err)
	}
}

// --- Bug 36: NodeService.DeleteNode route cleanup ---

func TestNodeService_DeleteNode_CleansRoutes(t *testing.T) {
	db := setupBugfix4TestDB(t)
	svc := NewNodeService(
		repository.NewNodeRepo(db),
		repository.NewNodeRouteRepo(db),
		nil,
		db,
	)

	// Create nodes
	_, _ = svc.CreateNode(context.Background(), &CreateNodeRequest{NodeID: "alice", Name: "Alice"})
	_, _ = svc.CreateNode(context.Background(), &CreateNodeRequest{NodeID: "bob", Name: "Bob"})
	_, _ = svc.CreateNode(context.Background(), &CreateNodeRequest{NodeID: "charlie", Name: "Charlie"})

	// Create routes: alice<->bob, alice<->charlie
	_ = svc.CreateRoute(context.Background(), &CreateRouteRequest{SrcNodeID: "alice", DstNodeID: "bob"})
	_ = svc.CreateRoute(context.Background(), &CreateRouteRequest{SrcNodeID: "alice", DstNodeID: "charlie"})

	// Verify routes exist
	var routeCount int64
	db.Model(&model.NodeRouteDO{}).Count(&routeCount)
	if routeCount != 4 { // 2 forward + 2 reverse
		t.Fatalf("expected 4 routes before delete, got %d", routeCount)
	}

	// Delete alice — should remove all routes involving alice
	if err := svc.DeleteNode(context.Background(), "alice"); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}

	// Verify no orphaned routes remain
	db.Model(&model.NodeRouteDO{}).Where("src_node_id = ? OR dst_node_id = ?", "alice", "alice").Count(&routeCount)
	if routeCount != 0 {
		t.Fatalf("expected 0 routes involving alice after delete, got %d", routeCount)
	}

	// Verify bob<->charlie routes are unaffected (there are none in this test,
	// but verify the total remaining is 0 since all routes involved alice)
	db.Model(&model.NodeRouteDO{}).Count(&routeCount)
	if routeCount != 0 {
		t.Fatalf("expected 0 total routes (all involved alice), got %d", routeCount)
	}
}

func TestNodeService_DeleteNode_NotFound(t *testing.T) {
	db := setupBugfix4TestDB(t)
	svc := NewNodeService(
		repository.NewNodeRepo(db),
		repository.NewNodeRouteRepo(db),
		nil,
		db,
	)

	err := svc.DeleteNode(context.Background(), "nonexistent")
	if err != ErrNodeNotFound {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}
