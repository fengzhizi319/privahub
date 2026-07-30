package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Vote service errors.
var (
	ErrVoteNotFound = errors.New("vote not found")
)

// VoteService handles multi-party vote/approval management.
type VoteService struct {
	voteRequestRepo repository.VoteRequestRepository
	voteInviteRepo  repository.VoteInviteRepository
	db              *gorm.DB
}

// NewVoteService creates a new VoteService.
func NewVoteService(
	voteRequestRepo repository.VoteRequestRepository,
	voteInviteRepo repository.VoteInviteRepository,
	db *gorm.DB,
) *VoteService {
	return &VoteService{
		voteRequestRepo: voteRequestRepo,
		voteInviteRepo:  voteInviteRepo,
		db:              db,
	}
}

// --- Request / Response DTOs ---

// CreateVoteRequest represents a vote creation request.
type CreateVoteRequest struct {
	Initiator         string   `json:"initiator" binding:"required"`
	Type              string   `json:"type" binding:"required"`
	Voters            []string `json:"voters" binding:"required"`
	Executors         []string `json:"executors"`
	ApprovedThreshold int      `json:"approved_threshold"`
	Description       string   `json:"description"`
	RequestMsg        string   `json:"request_msg"`
}

// VoteVO represents a vote view object.
type VoteVO struct {
	VoteID            string `json:"vote_id"`
	Initiator         string `json:"initiator"`
	Type              string `json:"type"`
	Voters            string `json:"voters"`
	VoteCounter       string `json:"vote_counter"`
	Executors         string `json:"executors"`
	ApprovedThreshold int    `json:"approved_threshold"`
	Status            int8   `json:"status"`
	ExecuteStatus     string `json:"execute_status"`
	Description       string `json:"description"`
	GmtCreate         string `json:"gmt_create"`
}

// VoteInviteVO represents a vote invite view object.
type VoteInviteVO struct {
	VoteID            string `json:"vote_id"`
	Initiator         string `json:"initiator"`
	VoteParticipantID string `json:"vote_participant_id"`
	Type              string `json:"type"`
	Action            string `json:"action"`
	Reason            string `json:"reason"`
	Description       string `json:"description"`
	GmtCreate         string `json:"gmt_create"`
}

// ListVoteRequest represents a vote list request.
type ListVoteRequest struct {
	Initiator string `json:"initiator"`
	Voter     string `json:"voter"`
}

// GetVoteRequest represents a vote detail request.
type GetVoteRequest struct {
	VoteID string `json:"vote_id" binding:"required"`
}

// ReplyVoteRequest represents a vote reply request.
type ReplyVoteRequest struct {
	VoteID            string `json:"vote_id" binding:"required"`
	VoteParticipantID string `json:"vote_participant_id" binding:"required"`
	Action            string `json:"action" binding:"required"` // AGREE / REJECT
	Reason            string `json:"reason"`
}

// --- Service Methods ---

// CreateVote creates a new vote request with invites atomically.
func (s *VoteService) CreateVote(ctx context.Context, req *CreateVoteRequest) (*VoteVO, error) {
	voteID := uuid.New().String()[:8]

	// Use json.Marshal to safely serialize voter/executor arrays (prevents JSON injection)
	votersBytes, err := json.Marshal(req.Voters)
	if err != nil {
		return nil, errors.New("failed to serialize voters")
	}
	votersJSON := string(votersBytes)

	executorsBytes, err := json.Marshal(req.Executors)
	if err != nil {
		return nil, errors.New("failed to serialize executors")
	}
	executorsJSON := string(executorsBytes)

	threshold := req.ApprovedThreshold
	if threshold == 0 {
		threshold = len(req.Voters)
	}

	vote := &model.VoteRequestDO{
		VoteID:            voteID,
		Initiator:         req.Initiator,
		Type:              req.Type,
		Voters:            votersJSON,
		VoteCounter:       req.Initiator,
		Executors:         executorsJSON,
		ApprovedThreshold: threshold,
		RequestMsg:        req.RequestMsg,
		Status:            0, // PENDING
		ExecuteStatus:     "COMMITTED",
		Description:       req.Description,
	}

	// Create vote and invites atomically in a transaction.
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(vote).Error; err != nil {
			return err
		}
		for _, voter := range req.Voters {
			invite := &model.VoteInviteDO{
				VoteID:            voteID,
				Initiator:         req.Initiator,
				VoteParticipantID: voter,
				Type:              req.Type,
				Action:            "REVIEWING",
				Description:       req.Description,
			}
			if err := tx.Create(invite).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &VoteVO{
		VoteID:            voteID,
		Initiator:         req.Initiator,
		Type:              req.Type,
		Voters:            votersJSON,
		VoteCounter:       req.Initiator,
		Executors:         executorsJSON,
		ApprovedThreshold: threshold,
		Status:            0,
		ExecuteStatus:     "COMMITTED",
		Description:       req.Description,
		GmtCreate:         vote.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// ListVotes lists votes by initiator or voter.
func (s *VoteService) ListVotes(ctx context.Context, req *ListVoteRequest) ([]VoteVO, error) {
	var votes []model.VoteRequestDO
	var err error

	if req.Initiator != "" {
		votes, err = s.voteRequestRepo.FindByInitiator(ctx, req.Initiator)
	} else if req.Voter != "" {
		votes, err = s.voteRequestRepo.FindByVoter(ctx, req.Voter)
	} else {
		// Return all votes (admin view)
		votes, err = s.voteRequestRepo.FindAll(ctx)
	}
	if err != nil {
		return nil, err
	}

	result := make([]VoteVO, 0, len(votes))
	for _, v := range votes {
		result = append(result, VoteVO{
			VoteID:            v.VoteID,
			Initiator:         v.Initiator,
			Type:              v.Type,
			Voters:            v.Voters,
			VoteCounter:       v.VoteCounter,
			Executors:         v.Executors,
			ApprovedThreshold: v.ApprovedThreshold,
			Status:            v.Status,
			ExecuteStatus:     v.ExecuteStatus,
			Description:       v.Description,
			GmtCreate:         v.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// GetVoteDetail retrieves vote detail with invites.
func (s *VoteService) GetVoteDetail(ctx context.Context, req *GetVoteRequest) (*VoteVO, []VoteInviteVO, error) {
	vote, err := s.voteRequestRepo.FindByVoteID(ctx, req.VoteID)
	if err != nil {
		return nil, nil, ErrVoteNotFound
	}

	invites, err := s.voteInviteRepo.FindByVoteID(ctx, req.VoteID)
	if err != nil {
		invites = nil
	}

	vo := &VoteVO{
		VoteID:            vote.VoteID,
		Initiator:         vote.Initiator,
		Type:              vote.Type,
		Voters:            vote.Voters,
		VoteCounter:       vote.VoteCounter,
		Executors:         vote.Executors,
		ApprovedThreshold: vote.ApprovedThreshold,
		Status:            vote.Status,
		ExecuteStatus:     vote.ExecuteStatus,
		Description:       vote.Description,
		GmtCreate:         vote.GmtCreate.Format("2006-01-02 15:04:05"),
	}

	inviteVOs := make([]VoteInviteVO, 0, len(invites))
	for _, inv := range invites {
		inviteVOs = append(inviteVOs, VoteInviteVO{
			VoteID:            inv.VoteID,
			Initiator:         inv.Initiator,
			VoteParticipantID: inv.VoteParticipantID,
			Type:              inv.Type,
			Action:            inv.Action,
			Reason:            inv.Reason,
			Description:       inv.Description,
			GmtCreate:         inv.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}

	return vo, inviteVOs, nil
}

// ReplyVote processes a vote reply (AGREE/REJECT) atomically to prevent race conditions.
func (s *VoteService) ReplyVote(ctx context.Context, req *ReplyVoteRequest) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find and update the invite within the transaction.
		var invite model.VoteInviteDO
		if err := tx.Where("vote_id = ? AND vote_participant_id = ?", req.VoteID, req.VoteParticipantID).First(&invite).Error; err != nil {
			return ErrVoteNotFound
		}

		invite.Action = req.Action
		invite.Reason = req.Reason
		if err := tx.Save(&invite).Error; err != nil {
			return err
		}

		// Count votes within the same transaction for consistency.
		var invites []model.VoteInviteDO
		if err := tx.Where("vote_id = ?", req.VoteID).Find(&invites).Error; err != nil {
			return nil // non-fatal: invite updated, skip threshold check
		}

		agreeCount := 0
		rejectCount := 0
		for _, inv := range invites {
			if inv.Action == "AGREE" {
				agreeCount++
			} else if inv.Action == "REJECT" {
				rejectCount++
			}
		}

		var vote model.VoteRequestDO
		if err := tx.Where("vote_id = ?", req.VoteID).First(&vote).Error; err != nil {
			return nil // non-fatal
		}

		if agreeCount >= vote.ApprovedThreshold {
			vote.Status = 1 // APPROVED
			return tx.Save(&vote).Error
		} else if rejectCount > len(invites)-vote.ApprovedThreshold {
			vote.Status = 2 // REJECTED
			return tx.Save(&vote).Error
		}

		return nil
	})
}
