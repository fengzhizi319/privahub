package votehandler

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// CompensatePartyVoteInfo mirrors Java P2pPaddingNodeServiceImpl.compensate:
// when the initiator's database sees a vote_invite change (typically a peer's
// reply applied by the P2P data sync), the matching entry of
// vote_request.party_vote_info takes over the invite's action and reason.
// Parties absent from party_vote_info are ignored like Java; an unchanged
// entry is not rewritten (avoids a redundant vote_request sync round).
// It reports whether vote_request was updated.
func CompensatePartyVoteInfo(ctx context.Context, db *gorm.DB, inv *model.VoteInviteDO) (bool, error) {
	if db == nil || inv == nil || inv.VoteID == "" || inv.VoteParticipantID == "" {
		return false, nil
	}
	var updated bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var vr model.VoteRequestDO
		if tx.Where("vote_id = ?", inv.VoteID).Limit(1).Find(&vr).RowsAffected == 0 {
			return nil
		}
		infos := parsePartyVoteInfos(vr.PartyVoteInfo)
		for i := range infos {
			if infos[i].PartyID != inv.VoteParticipantID {
				continue
			}
			if infos[i].Action == inv.Action && infos[i].Reason == inv.Reason {
				return nil
			}
			infos[i].Action = inv.Action
			infos[i].Reason = inv.Reason
			// Model(&vr) keeps the row's key so the sync producer emits the update.
			if err := tx.Model(&vr).Update("party_vote_info", toJSON(infos)).Error; err != nil {
				return err
			}
			updated = true
			return nil
		}
		return nil
	})
	return updated, err
}
