package datasync

import (
	"context"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/config"
)

// Options configures the Manager.
type Options struct {
	DB  *gorm.DB
	Cfg config.SyncConfig
	Env *service.EnvService
	// Sse is the CENTER SSE server (may be nil on other platforms).
	Sse *service.SseServer
	// Gateway is kuscia.gateway; CenterHost is server.center_platform_service.
	Gateway    string
	CenterHost string
	Log        *zap.Logger
}

// Manager wires the sync components for the active platform.
type Manager struct {
	Registry *Registry
	Tables   TableSet
	// Consumer serves /api/v1alpha1/data/sync.
	Consumer *Consumer
	// VoteSync serves /api/v1alpha1/vote_sync/create.
	VoteSync *VoteSyncService
	// Sender is the service.DbSyncSender / PeerPoster.
	Sender *GatewaySender
	// Producer is the GORM change capture (CENTER/AUTONOMY when enabled).
	Producer *Producer
	// Center dispatches changes over SSE (CENTER).
	Center *CenterDispatcher
	// P2P queues changes to peers (AUTONOMY).
	P2P *P2PSync
	// Edge is the SSE client (EDGE).
	Edge *EdgeClient

	async *AsyncSink

	env *service.EnvService
	log *zap.Logger
}

// envIdentity is the LocalIdentity backed by EnvService + node table.
type envIdentity struct {
	db  *gorm.DB
	env *service.EnvService
}

// NewEnvIdentity returns the LocalIdentity of env (local inst id and the
// node ids of the local institution).
func NewEnvIdentity(db *gorm.DB, env *service.EnvService) LocalIdentity {
	return envIdentity{db: db, env: env}
}

func (e envIdentity) InstID() string { return e.env.GetInstID() }

func (e envIdentity) NodeIDs() []string {
	ids := []string{e.env.GetPlatformNodeId()}
	if inst := e.env.GetInstID(); inst != "" && e.db != nil {
		var more []string
		e.db.Model(&model.NodeDO{}).Where("inst_id = ?", inst).Pluck("node_id", &more)
		ids = append(ids, more...)
	}
	return compact(ids)
}

// DefaultTables returns the Java data.sync default of a platform.
func DefaultTables(p service.PlatformType) []string {
	if p == service.PlatformAutonomy {
		return DefaultP2PTables
	}
	return DefaultCenterTables
}

// NewManager builds the components; nothing runs until Start.
func NewManager(o Options) (*Manager, error) {
	log := o.Log
	if log == nil {
		log = zap.NewNop()
	}
	env := o.Env
	if env == nil {
		env = service.GlobalEnv()
	}
	reg := DefaultRegistry()
	names := o.Cfg.Tables
	if len(names) == 0 {
		names = DefaultTables(env.GetPlatformType())
	}
	tables, unknown := reg.ResolveTables(names)
	if len(unknown) > 0 {
		log.Warn("data sync: unknown tables ignored", zap.Strings("tables", unknown))
	}
	m := &Manager{Registry: reg, Tables: tables, env: env, log: log}
	nodeID := env.GetPlatformNodeId()
	m.Consumer = NewConsumer(o.DB, reg, tables, nodeID, log).
		SetLocalIdentity(NewEnvIdentity(o.DB, env), env.IsAutonomy())
	m.VoteSync = NewVoteSyncService(o.DB, reg, log)
	m.Sender = NewGatewaySender(o.DB, reg, GatewaySenderOptions{
		Gateway: o.Gateway, CenterHost: o.CenterHost, LocalNodeID: nodeID,
		Timeout: o.Cfg.Timeout(), PlatformType: env.GetPlatformType(),
	})

	switch env.GetPlatformType() {
	case service.PlatformCenter:
		if o.Sse != nil {
			o.Sse.SetTableFilter(func(name string) (string, bool) {
				t, ok := reg.Canonical(name)
				return t, ok && tables.Has(t)
			})
			if o.Cfg.CenterEnabled() {
				m.Center = NewCenterDispatcher(o.DB, reg, o.Sse, tables, o.Cfg.Dispatch(), o.Cfg.Retries(), log)
				o.Sse.SetOpenHook(m.Center.InitialSync)
				m.Producer = NewProducer(reg, tables, m.Center.Push, log)
			}
		}
	case service.PlatformAutonomy:
		if o.Cfg.P2PEnabled() {
			m.P2P = NewP2PSync(o.DB, reg, tables, envIdentity{db: o.DB, env: env}, m.Sender,
				o.Cfg.QueueDir(), o.Cfg.Retries(), o.Cfg.Dispatch(), log)
			// P2P filtering/padding queries the DB: run it off the writer's
			// transaction (Java pushes @Async too).
			m.async = NewAsyncSink(m.P2P.Push, 0)
			m.Producer = NewProducer(reg, tables, m.async.Push, log)
		}
	case service.PlatformEdge:
		if o.Cfg.EdgeSyncEnabled() {
			m.Edge = NewEdgeClient(o.DB, reg, m.Consumer, EdgeClientOptions{
				Gateway: o.Gateway, CenterHost: o.CenterHost, NodeID: nodeID, Tables: tables,
				Reconnect: o.Cfg.EdgeReconnect(), IdleTimeout: o.Cfg.EdgeIdle(),
			}, log)
		}
	}
	if m.Producer != nil {
		if err := m.Producer.Register(o.DB); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// Start starts the background loops of the active platform.
func (m *Manager) Start() {
	if m.Center != nil {
		m.Center.Start()
	}
	if m.P2P != nil {
		m.P2P.InitOwnerCache(context.Background())
		m.P2P.Start()
	}
	if m.Edge != nil {
		m.Edge.Start()
	}
	m.log.Info("data sync started", zap.String("platform", string(m.env.GetPlatformType())),
		zap.Strings("tables", m.Tables.List()),
		zap.Bool("center_push", m.Center != nil), zap.Bool("p2p_push", m.P2P != nil), zap.Bool("edge_client", m.Edge != nil))
}

// Stop stops all loops.
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	if m.Edge != nil {
		m.Edge.Stop()
	}
	if m.async != nil {
		m.async.Stop()
	}
	if m.P2P != nil {
		m.P2P.Stop()
	}
	if m.Center != nil {
		m.Center.Stop()
	}
}
