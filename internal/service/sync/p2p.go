package datasync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/internal/service/votehandler"
)

// PeerPoster delivers one change to a peer (P2pDataSyncRestService.sync).
type PeerPoster interface {
	PostDataSync(ctx context.Context, dst string, d *service.SyncData) error
}

// LocalIdentity provides the local inst id and node ids (Java
// P2pDataSyncProducerTemplate.instId / nodeIds).
type LocalIdentity interface {
	InstID() string
	NodeIDs() []string
}

// P2PSync is the AUTONOMY producer: filter (Java P2pDataSyncProducerTemplate),
// destination padding (P2pPaddingNodeServiceImpl), per-destination queue
// persisted to disk (P2PDataSyncDataBufferTemplate) and the sender job with
// retries (DataSyncJob + P2pDataSyncRestTemplate).
type P2PSync struct {
	db         *gorm.DB
	reg        *Registry
	tables     TableSet
	id         LocalIdentity
	poster     PeerPoster
	dir        string
	maxRetries int
	interval   time.Duration
	log        *zap.Logger

	ownerMu sync.RWMutex
	owners  map[string]string // projectId / projectId_graphId -> owner inst

	qmu     sync.Mutex
	queues  map[string][]*ChangeEvent
	working map[string]bool

	started  atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewP2PSync creates the P2P producer/sender.
func NewP2PSync(db *gorm.DB, reg *Registry, tables TableSet, id LocalIdentity, poster PeerPoster,
	dir string, maxRetries int, interval time.Duration, log *zap.Logger) *P2PSync {
	if reg == nil {
		reg = DefaultRegistry()
	}
	if log == nil {
		log = zap.NewNop()
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}
	return &P2PSync{db: db, reg: reg, tables: tables, id: id, poster: poster, dir: dir,
		maxRetries: maxRetries, interval: interval, log: log,
		owners: map[string]string{}, queues: map[string][]*ChangeEvent{}, working: map[string]bool{},
		stopCh: make(chan struct{})}
}

// InitOwnerCache loads project/graph owners (Java P2pDataInit.init_ownerId_cache).
func (p *P2PSync) InitOwnerCache(ctx context.Context) {
	var projects []model.ProjectDO
	if err := p.db.WithContext(ctx).Find(&projects).Error; err == nil {
		for _, pr := range projects {
			p.setOwner(pr.ProjectID, pr.OwnerID)
		}
	}
	var graphs []model.ProjectGraphDO
	if err := p.db.WithContext(ctx).Find(&graphs).Error; err == nil {
		for _, g := range graphs {
			p.setOwner(g.ProjectID+"_"+g.GraphID, g.OwnerID)
		}
	}
}

func (p *P2PSync) setOwner(k, owner string) {
	p.ownerMu.Lock()
	p.owners[k] = owner
	p.ownerMu.Unlock()
}

// owner returns the cached owner, loading it from the DB on a miss.
func (p *P2PSync) owner(projectID, graphID string) string {
	k := projectID
	if graphID != "" {
		k = projectID + "_" + graphID
	}
	p.ownerMu.RLock()
	o, ok := p.owners[k]
	p.ownerMu.RUnlock()
	if ok {
		return o
	}
	if graphID != "" {
		var g model.ProjectGraphDO
		if p.db.Unscoped().Where("project_id = ? AND graph_id = ?", projectID, graphID).Limit(1).Find(&g).RowsAffected > 0 {
			o = g.OwnerID
		}
	} else {
		var pr model.ProjectDO
		if p.db.Unscoped().Where("project_id = ?", projectID).Limit(1).Find(&pr).RowsAffected > 0 {
			o = pr.OwnerID
		}
	}
	if o != "" {
		p.setOwner(k, o)
	}
	return o
}

func (p *P2PSync) instID() string {
	if p.id == nil {
		return ""
	}
	return p.id.InstID()
}

func (p *P2PSync) isLocalNode(n string) bool {
	if p.id == nil || n == "" {
		return false
	}
	for _, id := range p.id.NodeIDs() {
		if id == n {
			return true
		}
	}
	return false
}

// Filter mirrors P2pDataSyncProducerTemplate.filter: true = do not sync.
// It may narrow ev.Route.NodeIDs (vote invite replies go to the initiator).
func (p *P2PSync) Filter(ev *ChangeEvent) bool {
	v := reflect.ValueOf(ev.Entity)
	inst := p.instID()
	f := func(name string) string { return fieldString(v, name) }
	switch ev.Table {
	case TableProject:
		p.setOwner(f("ProjectID"), f("OwnerID"))
		return f("OwnerID") != inst
	case TableProjectGraph:
		p.setOwner(f("ProjectID")+"_"+f("GraphID"), f("OwnerID"))
		return f("OwnerID") != inst
	case TableProjectGraphNode, TableProjectGraphNodeKuscia:
		return p.owner(f("ProjectID"), f("GraphID")) != inst
	case TableProjectJob:
		owner := p.owner(f("ProjectID"), f("GraphID"))
		if ev.Action == service.SyncActionUpdate {
			if graphjob.NormalizeJobStatus(f("Status")) == graphjob.JobStopped {
				return owner != inst
			}
			return true
		}
		return owner != inst
	case TableProjectDatatable, TableProjectFeatureTable:
		return !p.isLocalNode(f("NodeID"))
	case TableProjectGraphDomainDs:
		return !p.isLocalNode(f("DomainID"))
	case TableProjectModelServing, TableProjectModelPack:
		return !p.isLocalNode(f("Initiator"))
	case TableVoteRequest, TableProjectApprovalConfig:
		return f("Initiator") != inst
	case TableVoteInvite:
		initiator := f("Initiator")
		if initiator == inst {
			// Java P2pPaddingNodeServiceImpl.compensate: the initiator's
			// vote_request party info follows the invite (peer replies).
			if inv, ok := ev.Entity.(*model.VoteInviteDO); ok && p.db != nil {
				if _, err := votehandler.CompensatePartyVoteInfo(context.Background(), p.db, inv); err != nil {
					p.log.Warn("compensate vote party info failed", zap.String("vote_id", inv.VoteID), zap.Error(err))
				}
			}
			return ev.Action != service.SyncActionCreate
		}
		if ev.Action == service.SyncActionUpdate {
			ev.Route.NodeIDs = []string{initiator}
			return false
		}
		return true
	case TableProjectNode, TableProjectInst:
		return p.owner(f("ProjectID"), "") != inst
	case TableProjectSchedule, TableProjectScheduleJob, TableProjectScheduleTask:
		return f("Owner") != inst
	}
	return false
}

// pad mirrors P2pPaddingNodeServiceImpl.paddingNodes: destinations are the
// vote parties plus the project insts and PROJECT_CREATE approval parties.
func (p *P2PSync) pad(ev *ChangeEvent) []string {
	var ids []string
	if ev.Table == TableVoteRequest || ev.Table == TableVoteInvite {
		ids = append(ids, ev.Route.NodeIDs...)
	}
	if pid := ev.Route.ProjectID; pid != "" {
		var insts []string
		p.db.Model(&model.ProjectInstDO{}).Where("project_id = ?", pid).Pluck("inst_id", &insts)
		ids = append(ids, insts...)
		var cfg model.ProjectApprovalConfigDO
		if p.db.Where("project_id = ? AND type = ?", pid, "PROJECT_CREATE").Limit(1).Find(&cfg).RowsAffected > 0 {
			ids = append(ids, ParseIDList(cfg.Parties)...)
			ids = append(ids, cfg.InviteNodeID)
		}
	}
	return compact(ids)
}

// Push is the producer sink (Java P2pDataSyncProducerTemplate.push).
func (p *P2PSync) Push(ev *ChangeEvent) {
	if ev == nil || !p.tables.Has(ev.Table) {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			p.log.Error("P2P data sync push panic", zap.Any("panic", r))
		}
	}()
	if p.Filter(ev) {
		return
	}
	p.enqueue(ev, p.pad(ev))
}

func (p *P2PSync) enqueue(ev *ChangeEvent, dsts []string) {
	inst := p.instID()
	for _, dst := range dsts {
		if dst == "" || dst == inst || p.isLocalNode(dst) {
			continue
		}
		cp := *ev
		cp.DstNode = dst
		p.qmu.Lock()
		q := p.queues[dst]
		// Pending updates of the same row are superseded by the newer one.
		if cp.Action == service.SyncActionUpdate && cp.Key != "" {
			kept := q[:0]
			for _, old := range q {
				if !(old.Action == service.SyncActionUpdate && old.Key == cp.Key) {
					kept = append(kept, old)
				}
			}
			q = kept
		}
		p.queues[dst] = append(q, &cp)
		p.persistLocked(dst)
		p.qmu.Unlock()
		p.trigger(dst)
	}
}

// Pending returns the queue length of a destination.
func (p *P2PSync) Pending(dst string) int {
	p.qmu.Lock()
	defer p.qmu.Unlock()
	return len(p.queues[dst])
}

func (p *P2PSync) queueFile(dst string) string {
	return filepath.Join(p.dir, filepath.Base(strings.ReplaceAll(dst, string(filepath.Separator), "_")))
}

// persistLocked writes the destination queue (qmu held).
func (p *P2PSync) persistLocked(dst string) {
	if p.dir == "" {
		return
	}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		p.log.Warn("create p2p sync dir failed", zap.Error(err))
		return
	}
	q := p.queues[dst]
	f := p.queueFile(dst)
	if len(q) == 0 {
		_ = os.Remove(f)
		return
	}
	b, err := json.Marshal(q)
	if err != nil {
		return
	}
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		_ = os.Rename(tmp, f)
	}
}

// Load restores persisted queues (Java P2PDataSyncDataBufferTemplate.init).
func (p *P2PSync) Load() {
	if p.dir == "" {
		return
	}
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return
	}
	for _, en := range entries {
		if en.IsDir() || strings.HasSuffix(en.Name(), ".tmp") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(p.dir, en.Name()))
		if err != nil {
			continue
		}
		var q []*ChangeEvent
		if json.Unmarshal(b, &q) != nil {
			continue
		}
		p.qmu.Lock()
		p.queues[en.Name()] = append(q, p.queues[en.Name()]...)
		p.qmu.Unlock()
	}
}

// Start restores the queues and runs the periodic sender.
func (p *P2PSync) Start() {
	p.Load()
	p.started.Store(true)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		t := time.NewTicker(p.interval)
		defer t.Stop()
		for {
			select {
			case <-p.stopCh:
				return
			case <-t.C:
				p.qmu.Lock()
				dsts := make([]string, 0, len(p.queues))
				for d, q := range p.queues {
					if len(q) > 0 {
						dsts = append(dsts, d)
					}
				}
				p.qmu.Unlock()
				for _, d := range dsts {
					p.trigger(d)
				}
			}
		}
	}()
}

// Stop stops the sender loop and waits for running workers.
func (p *P2PSync) Stop() {
	p.stopOnce.Do(func() { close(p.stopCh) })
	p.wg.Wait()
}

// trigger starts a worker for dst unless one is running (Java DataSyncJob).
// Before Start, changes only accumulate in the queues.
func (p *P2PSync) trigger(dst string) {
	if !p.started.Load() {
		return
	}
	select {
	case <-p.stopCh:
		return
	default:
	}
	p.qmu.Lock()
	if p.working[dst] {
		p.qmu.Unlock()
		return
	}
	p.working[dst] = true
	p.qmu.Unlock()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer func() {
			p.qmu.Lock()
			delete(p.working, dst)
			p.qmu.Unlock()
		}()
		p.Drain(dst)
	}()
}

// Drain sends the queue of dst in order. On failure the event is moved to
// the queue tail (up to maxRetries) and draining stops for this round.
func (p *P2PSync) Drain(dst string) {
	for {
		p.qmu.Lock()
		q := p.queues[dst]
		if len(q) == 0 {
			p.qmu.Unlock()
			return
		}
		ev := q[0]
		p.qmu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := p.poster.PostDataSync(ctx, dst, ev.SyncData())
		cancel()

		p.qmu.Lock()
		q = p.queues[dst]
		if len(q) > 0 && q[0] == ev {
			q = q[1:]
		}
		failed := false
		if err != nil {
			ev.Retries++
			if ev.Retries < p.maxRetries {
				p.log.Warn("data sync send error, retry", zap.String("dst", dst), zap.String("table", ev.Table), zap.Int("retry", ev.Retries), zap.Error(err))
				q = append(q, ev)
			} else {
				p.log.Error("data sync send error, drop", zap.String("dst", dst), zap.String("table", ev.Table), zap.Error(err))
			}
			failed = true
		}
		p.queues[dst] = q
		p.persistLocked(dst)
		p.qmu.Unlock()
		if failed {
			return
		}
	}
}
