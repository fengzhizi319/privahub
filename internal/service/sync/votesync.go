package datasync

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ErrEmptyVoteSync is returned for a request without dbSyncRequests.
var ErrEmptyVoteSync = errors.New("dbSyncRequest can not be empty")

// VoteSyncService implements /api/v1alpha1/vote_sync/create (Java
// VoteSyncServiceImpl): upsert of the 7 vote-sync entity types. The writes are
// NOT marked as consumer writes, so a CENTER re-publishes them to the other
// edges like Java (JPA listeners fire on save).
type VoteSyncService struct {
	db       *gorm.DB
	reg      *Registry
	consumer *Consumer
	log      *zap.Logger
}

// NewVoteSyncService creates the service.
func NewVoteSyncService(db *gorm.DB, reg *Registry, log *zap.Logger) *VoteSyncService {
	if reg == nil {
		reg = DefaultRegistry()
	}
	if log == nil {
		log = zap.NewNop()
	}
	allowed := TableSet{}
	for _, t := range voteSyncTypes {
		allowed[t] = true
	}
	return &VoteSyncService{db: db, reg: reg, log: log, consumer: NewConsumer(db, reg, allowed, "", log)}
}

// Sync applies the requests in one transaction.
func (s *VoteSyncService) Sync(ctx context.Context, reqs []DbSyncRequest) error {
	if len(reqs) == 0 {
		return ErrEmptyVoteSync
	}
	type item struct {
		table  string
		entity interface{}
	}
	items := make([]item, 0, len(reqs))
	for i, r := range reqs {
		table, ok := VoteSyncTableOf(r.SyncDataType)
		if !ok {
			return fmt.Errorf("dbSyncRequests[%d]: unsupported syncDataType %q", i, r.SyncDataType)
		}
		entity, err := s.reg.Decode(table, r.ProjectNodesInfo)
		if err != nil {
			return fmt.Errorf("dbSyncRequests[%d]: %w", i, err)
		}
		items = append(items, item{table: table, entity: entity})
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c := NewConsumer(tx, s.reg, s.consumer.allowed, "", s.log)
		for _, it := range items {
			if err := c.Upsert(ctx, it.table, it.entity); err != nil {
				return fmt.Errorf("sync %s: %w", it.table, err)
			}
		}
		return nil
	})
}
