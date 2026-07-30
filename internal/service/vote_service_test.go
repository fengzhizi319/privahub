package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVoteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.VoteRequestDO{},
		&model.VoteInviteDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestVoteService_CreateVote_Success(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	resp, err := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator:   "alice",
		Type:        "PROJECT_CREATE",
		Voters:      []string{"bob", "carol"},
		Executors:   []string{"alice"},
		Description: "Test project creation",
		RequestMsg:  "Please approve",
	})
	if err != nil {
		t.Fatalf("CreateVote failed: %v", err)
	}
	if resp.VoteID == "" {
		t.Error("expected non-empty vote ID")
	}
	if resp.Initiator != "alice" {
		t.Errorf("expected initiator 'alice', got %q", resp.Initiator)
	}
	if resp.ApprovedThreshold != 2 {
		t.Errorf("expected threshold 2 (default to voter count), got %d", resp.ApprovedThreshold)
	}
	if resp.Status != 0 {
		t.Errorf("expected status 0 (PENDING), got %d", resp.Status)
	}

	// Verify invites were created
	var invites []model.VoteInviteDO
	db.Where("vote_id = ?", resp.VoteID).Find(&invites)
	if len(invites) != 2 {
		t.Errorf("expected 2 invites, got %d", len(invites))
	}
}

func TestVoteService_CreateVote_CustomThreshold(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	resp, err := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator:         "alice",
		Type:              "DATA_GRANT",
		Voters:            []string{"bob", "carol", "dave"},
		ApprovedThreshold: 2, // Only need 2 out of 3
		Description:       "Data grant approval",
	})
	if err != nil {
		t.Fatalf("CreateVote failed: %v", err)
	}
	if resp.ApprovedThreshold != 2 {
		t.Errorf("expected threshold 2, got %d", resp.ApprovedThreshold)
	}
}

func TestVoteService_ListVotes_ByInitiator(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	// Create votes
	svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator: "alice",
		Type:      "PROJECT_CREATE",
		Voters:    []string{"bob"},
	})
	svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator: "bob",
		Type:      "DATA_GRANT",
		Voters:    []string{"alice"},
	})

	// List by initiator
	votes, err := svc.ListVotes(context.Background(), &ListVoteRequest{Initiator: "alice"})
	if err != nil {
		t.Fatalf("ListVotes failed: %v", err)
	}
	if len(votes) != 1 {
		t.Errorf("expected 1 vote by alice, got %d", len(votes))
	}
	if len(votes) > 0 && votes[0].Initiator != "alice" {
		t.Errorf("expected initiator 'alice', got %q", votes[0].Initiator)
	}
}

func TestVoteService_ListVotes_ByVoter(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator: "alice",
		Type:      "PROJECT_CREATE",
		Voters:    []string{"bob", "carol"},
	})

	votes, err := svc.ListVotes(context.Background(), &ListVoteRequest{Voter: "bob"})
	if err != nil {
		t.Fatalf("ListVotes failed: %v", err)
	}
	if len(votes) != 1 {
		t.Errorf("expected 1 vote for voter bob, got %d", len(votes))
	}
}

func TestVoteService_GetVoteDetail_Success(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	created, _ := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator:   "alice",
		Type:        "PROJECT_CREATE",
		Voters:      []string{"bob", "carol"},
		Description: "Test vote",
	})

	vote, invites, err := svc.GetVoteDetail(context.Background(), &GetVoteRequest{VoteID: created.VoteID})
	if err != nil {
		t.Fatalf("GetVoteDetail failed: %v", err)
	}
	if vote.VoteID != created.VoteID {
		t.Errorf("expected vote ID %q, got %q", created.VoteID, vote.VoteID)
	}
	if len(invites) != 2 {
		t.Errorf("expected 2 invites, got %d", len(invites))
	}
}

func TestVoteService_GetVoteDetail_NotFound(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	_, _, err := svc.GetVoteDetail(context.Background(), &GetVoteRequest{VoteID: "nonexistent"})
	if err != ErrVoteNotFound {
		t.Errorf("expected ErrVoteNotFound, got %v", err)
	}
}

func TestVoteService_ReplyVote_Approve(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	created, _ := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator:         "alice",
		Type:              "PROJECT_CREATE",
		Voters:            []string{"bob"},
		ApprovedThreshold: 1,
	})

	// Bob approves
	err := svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            created.VoteID,
		VoteParticipantID: "bob",
		Action:            "AGREE",
		Reason:            "Looks good",
	})
	if err != nil {
		t.Fatalf("ReplyVote failed: %v", err)
	}

	// Verify vote status is APPROVED
	vote, _, _ := svc.GetVoteDetail(context.Background(), &GetVoteRequest{VoteID: created.VoteID})
	if vote.Status != 1 {
		t.Errorf("expected status 1 (APPROVED), got %d", vote.Status)
	}
}

func TestVoteService_ReplyVote_RejectVote(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	created, _ := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator:         "alice",
		Type:              "PROJECT_CREATE",
		Voters:            []string{"bob"},
		ApprovedThreshold: 1,
	})

	// Bob rejects
	err := svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            created.VoteID,
		VoteParticipantID: "bob",
		Action:            "REJECT",
		Reason:            "Not appropriate",
	})
	if err != nil {
		t.Fatalf("ReplyVote failed: %v", err)
	}

	// Verify vote status is REJECTED
	vote, _, _ := svc.GetVoteDetail(context.Background(), &GetVoteRequest{VoteID: created.VoteID})
	if vote.Status != 2 {
		t.Errorf("expected status 2 (REJECTED), got %d", vote.Status)
	}
}

func TestVoteService_ReplyVote_MultiPartyThreshold(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	created, _ := svc.CreateVote(context.Background(), &CreateVoteRequest{
		Initiator:         "alice",
		Type:              "PROJECT_CREATE",
		Voters:            []string{"bob", "carol", "dave"},
		ApprovedThreshold: 2, // Need 2 out of 3
	})

	// Bob approves - should still be PENDING (1 < 2)
	svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            created.VoteID,
		VoteParticipantID: "bob",
		Action:            "AGREE",
	})
	vote, _, _ := svc.GetVoteDetail(context.Background(), &GetVoteRequest{VoteID: created.VoteID})
	if vote.Status != 0 {
		t.Errorf("expected status 0 (PENDING) after 1 approval, got %d", vote.Status)
	}

	// Carol approves - should be APPROVED (2 >= 2)
	svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            created.VoteID,
		VoteParticipantID: "carol",
		Action:            "AGREE",
	})
	vote, _, _ = svc.GetVoteDetail(context.Background(), &GetVoteRequest{VoteID: created.VoteID})
	if vote.Status != 1 {
		t.Errorf("expected status 1 (APPROVED) after 2 approvals, got %d", vote.Status)
	}
}

func TestVoteService_ReplyVote_VoteNotFound(t *testing.T) {
	db := setupVoteTestDB(t)
	svc := NewVoteService(
		repository.NewVoteRequestRepo(db),
		repository.NewVoteInviteRepo(db),
		db,
	)

	err := svc.ReplyVote(context.Background(), &ReplyVoteRequest{
		VoteID:            "nonexistent",
		VoteParticipantID: "bob",
		Action:            "AGREE",
	})
	if err != ErrVoteNotFound {
		t.Errorf("expected ErrVoteNotFound, got %v", err)
	}
}
