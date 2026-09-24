package service

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// KusciaRouter routes KusciaAPI calls to the per-node client of the target
// domain (Java DynamicKusciaChannelProvider.createStub(domainId, ...)), with
// the default client as fallback when the domain has no dedicated channel:
//
//   - domain-scoped calls (DomainData / Grant / DataSource / Domain / Route)
//     use the client of the domain named in the request;
//   - CreateJob uses the client of the job initiator; the initiator is
//     remembered so that later Query / Stop / Delete / Restart of the job go
//     to the same KusciaAPI;
//   - job calls for an unknown job resolve the job's parties from
//     project_job_task (Kuscia replicates a job to every party) and pick the
//     first party with a dedicated channel.
//
// With an empty pool every call goes to the default client, i.e. the
// single-KusciaAPI (CENTER) behaviour is unchanged.
type KusciaRouter struct {
	pool *kuscia.ClientPool
	def  *kuscia.Client
	db   *gorm.DB

	mu       sync.RWMutex
	jobNodes map[string]string // job id -> domain id of the channel used
}

// NewKusciaRouter creates a router over pool (default: pool.Default(), or def
// when the pool is nil).
func NewKusciaRouter(pool *kuscia.ClientPool, def *kuscia.Client, db *gorm.DB) *KusciaRouter {
	if pool != nil && pool.Default() != nil {
		def = pool.Default()
	}
	return &KusciaRouter{pool: pool, def: def, db: db, jobNodes: map[string]string{}}
}

var (
	_ GraphKusciaAPI       = (*KusciaRouter)(nil)
	_ ScheduleKusciaClient = (*KusciaRouter)(nil)
)

// ForNode returns the client for domainID (dedicated channel or default).
func (r *KusciaRouter) ForNode(domainID string) *kuscia.Client {
	if r.pool != nil {
		if c, ok := r.pool.Lookup(domainID); ok && c != nil {
			return c
		}
	}
	return r.def
}

// Default returns the default client.
func (r *KusciaRouter) Default() *kuscia.Client { return r.def }

// NodeForJob returns the domain whose channel serves jobID ("" = default).
func (r *KusciaRouter) NodeForJob(ctx context.Context, jobID string) string {
	r.mu.RLock()
	node, ok := r.jobNodes[jobID]
	r.mu.RUnlock()
	if ok {
		return node
	}
	if r.pool == nil || r.db == nil || len(r.pool.Endpoints()) == 0 {
		return ""
	}
	var tasks []model.ProjectJobTaskDO
	if err := r.db.WithContext(ctx).Select("parties").Where("job_id = ?", jobID).Find(&tasks).Error; err != nil {
		return ""
	}
	for _, t := range tasks {
		var parties []string
		if json.Unmarshal([]byte(t.Parties), &parties) != nil {
			continue
		}
		for _, p := range parties {
			if r.pool.IsRegistered(p) {
				r.remember(jobID, p)
				return p
			}
		}
	}
	return ""
}

func (r *KusciaRouter) remember(jobID, node string) {
	if jobID == "" {
		return
	}
	r.mu.Lock()
	if len(r.jobNodes) > 10000 {
		r.jobNodes = map[string]string{} // bounded cache; DB lookup refills it
	}
	r.jobNodes[jobID] = node
	r.mu.Unlock()
}

func (r *KusciaRouter) forJob(ctx context.Context, jobID string) *kuscia.Client {
	return r.ForNode(r.NodeForJob(ctx, jobID))
}

// --- jobs ---

// CreateJob submits the job to the initiator's KusciaAPI.
func (r *KusciaRouter) CreateJob(ctx context.Context, req *kuscia.CreateJobRequest) (*kuscia.CreateJobResponse, error) {
	node := ""
	if req != nil && r.pool != nil && r.pool.IsRegistered(req.Initiator) {
		node = req.Initiator
	}
	resp, err := r.ForNode(node).CreateJob(ctx, req)
	if err == nil && req != nil {
		r.remember(req.JobID, node)
	}
	return resp, err
}

// StopJob stops the job on the KusciaAPI that serves it.
func (r *KusciaRouter) StopJob(ctx context.Context, jobID string) error {
	return r.forJob(ctx, jobID).StopJob(ctx, jobID)
}

// DeleteJob deletes the job on the KusciaAPI that serves it.
func (r *KusciaRouter) DeleteJob(ctx context.Context, jobID string) error {
	return r.forJob(ctx, jobID).DeleteJob(ctx, jobID)
}

// RestartJob restarts the job on the KusciaAPI that serves it.
func (r *KusciaRouter) RestartJob(ctx context.Context, jobID string) error {
	return r.forJob(ctx, jobID).RestartJob(ctx, jobID)
}

// QueryJob queries the job on the KusciaAPI that serves it.
func (r *KusciaRouter) QueryJob(ctx context.Context, jobID string) (*kuscia.QueryJobResponse, error) {
	return r.forJob(ctx, jobID).QueryJob(ctx, jobID)
}

// QueryJobDetail queries the job detail on the KusciaAPI that serves it.
func (r *KusciaRouter) QueryJobDetail(ctx context.Context, jobID string) (*kuscia.QueryJobDetailResponse, error) {
	return r.forJob(ctx, jobID).QueryJobDetail(ctx, jobID)
}

// --- domain-scoped ---

// QueryDomainData uses the channel of the owning domain.
func (r *KusciaRouter) QueryDomainData(ctx context.Context, domainID, domainDataID string) (*kuscia.QueryDomainDataResponse, error) {
	return r.ForNode(domainID).QueryDomainData(ctx, domainID, domainDataID)
}

// CreateDomainDataGrantWithID uses the channel of the granting domain.
func (r *KusciaRouter) CreateDomainDataGrantWithID(ctx context.Context, req *kuscia.DomainDataGrantWithIDRequest) error {
	node := ""
	if req != nil {
		node = req.DomainID
	}
	return r.ForNode(node).CreateDomainDataGrantWithID(ctx, req)
}

// QueryDomainDataGrantByID uses the channel of the granting domain.
func (r *KusciaRouter) QueryDomainDataGrantByID(ctx context.Context, domainID, grantID string) error {
	return r.ForNode(domainID).QueryDomainDataGrantByID(ctx, domainID, grantID)
}

// ListDomainDataSource uses the channel of the domain.
func (r *KusciaRouter) ListDomainDataSource(ctx context.Context, domainID string) ([]kuscia.DomainDataSource, error) {
	return r.ForNode(domainID).ListDomainDataSource(ctx, domainID)
}

// QueryDomain uses the channel of the domain.
func (r *KusciaRouter) QueryDomain(ctx context.Context, domainID string) (*kuscia.QueryDomainResponse, error) {
	return r.ForNode(domainID).QueryDomain(ctx, domainID)
}

// QueryDomainRoute uses the channel of the route source domain.
func (r *KusciaRouter) QueryDomainRoute(ctx context.Context, source, destination string) (*kuscia.QueryDomainRouteResponse, error) {
	return r.ForNode(source).QueryDomainRoute(ctx, source, destination)
}
