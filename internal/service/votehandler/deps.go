package votehandler

import (
	"context"
	"errors"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DbSyncSender pushes data changes from an EDGE platform to the center (Java
// DbSyncUtil.dbDataSyncToCenter). When none is set explicitly, the
// process-wide service.CurrentDbSyncSender() (installed by the sync
// subsystem) is used through GlobalDbSyncSender.
type DbSyncSender interface {
	SendToCenter(ctx context.Context, reqs ...DbSyncRequest) error
}

// syncTables maps Java VoteSyncTypeEnum values to Go table names.
var syncTables = map[string]string{
	SyncVoteRequest:           "vote_request",
	SyncVoteInvite:            "vote_invite",
	SyncProjectApprovalConfig: "project_approval_config",
	SyncProject:               "project",
	SyncProjectNode:           "project_node",
	SyncProjectInst:           "project_inst",
}

// GlobalDbSyncSender adapts service.CurrentDbSyncSender() (Send(ctx, nodeID,
// ...SyncData); nodeID "" = center) to DbSyncSender.
type GlobalDbSyncSender struct{}

// SendToCenter implements DbSyncSender.
func (GlobalDbSyncSender) SendToCenter(ctx context.Context, reqs ...DbSyncRequest) error {
	entities := make([]service.SyncData, 0, len(reqs))
	for _, r := range reqs {
		table, ok := syncTables[r.SyncDataType]
		if !ok {
			return errors.New("unsupported sync data type " + r.SyncDataType)
		}
		entities = append(entities, service.SyncData{TableName: table, Action: service.SyncActionUpdate, Data: r.ProjectNodesInfo})
	}
	return service.CurrentDbSyncSender().Send(ctx, "", entities...)
}

// TeeResultPuller pulls a TEE result once its download vote is approved (Java
// DatatableService.pullResultFromTeeNode). When nil the approval is only
// recorded (vote execute status SUCCESS).
type TeeResultPuller interface {
	PullResultFromTeeNode(ctx context.Context, nodeID, resourceID, voteResult, projectID, jobID, taskID, resourceType string) error
}

// RouteCreator creates a node route (NodeRouteService.CreateRoute).
type RouteCreator interface {
	CreateRoute(ctx context.Context, req *service.CreateRouteRequest) error
}

// KusciaOps are the KusciaAPI operations used by the vote system. The
// channel selects the per-node KusciaAPI (empty = default).
type KusciaOps interface {
	// GenerateKeyCerts issues a vote key/cert pair via the KusciaAPI of nodeID
	// (Java CertificateService.generateCertByNodeID, common name "vote").
	GenerateKeyCerts(ctx context.Context, nodeID string) (*service.KeyCerts, error)
	// DomainCert returns the domain certificate of nodeID ("" when unknown;
	// Java NodeManager.getCert / getCerts).
	DomainCert(ctx context.Context, nodeID, channel string) string
	CreatePartnerDomain(ctx context.Context, channel, domainID, cert string) error
	DeleteDomain(ctx context.Context, channel, domainID string) error
	DomainRouteExists(ctx context.Context, channel, src, dst string) (bool, error)
	DomainRouteReady(ctx context.Context, channel, src, dst string) bool
	DeleteDomainRoute(ctx context.Context, channel, src, dst string) error
}

// Deps are the collaborators shared by all vote handlers and services.
type Deps struct {
	DB  *gorm.DB
	Env *service.EnvService
	// Kuscia may be nil (offline mode): signing and signature verification
	// are then skipped and no Kuscia resources are created.
	Kuscia    KusciaOps
	Routes    RouteCreator
	Sync      DbSyncSender
	TeePuller TeeResultPuller
	Log       *zap.Logger
}

func (d *Deps) log() *zap.Logger {
	if d.Log == nil {
		return zap.NewNop()
	}
	return d.Log
}

func (d *Deps) env() *service.EnvService {
	if d.Env != nil {
		return d.Env
	}
	if g := service.GlobalEnv(); g != nil {
		return g
	}
	return service.NewEnvService("center", "", "", nil)
}

func (d *Deps) isAutonomy() bool { return d.env().IsAutonomy() }
func (d *Deps) isCenter() bool   { return d.env().IsCenter() }
func (d *Deps) isEdge() bool     { return d.env().IsEdge() }

// platformNodeID is Java envService.getPlatformNodeId().
func (d *Deps) platformNodeID() string { return d.env().GetPlatformNodeId() }

// currentInstID is Java InstServiceImpl.INST_ID: the configured inst id, or
// the inst of the platform node.
func (d *Deps) currentInstID(ctx context.Context) string {
	if id := d.env().GetInstID(); id != "" {
		return id
	}
	var n model.NodeDO
	if err := d.DB.WithContext(ctx).Where("node_id = ?", d.platformNodeID()).First(&n).Error; err == nil {
		return n.InstID
	}
	return ""
}

func (d *Deps) isCurrentInst(ctx context.Context, instID string) bool {
	return instID != "" && instID == d.currentInstID(ctx)
}

// --- default KusciaOps backed by NodeService's client pool ---

type poolKuscia struct {
	ns    *service.NodeService
	certs *service.CertificateService
	def   *kuscia.Client
}

// NewKusciaOps returns KusciaOps using the node client pool (per-node
// channels) and the CertificateService. It returns nil when no Kuscia client
// is configured.
func NewKusciaOps(ns *service.NodeService, certs *service.CertificateService, def *kuscia.Client) KusciaOps {
	if def == nil && (ns == nil || ns.Pool() == nil || ns.Pool().Default() == nil) {
		return nil
	}
	return &poolKuscia{ns: ns, certs: certs, def: def}
}

func (p *poolKuscia) client(channel string) *kuscia.Client {
	if p.ns != nil {
		if pool := p.ns.Pool(); pool != nil {
			if c := pool.Get(channel); c != nil {
				return c
			}
		}
	}
	return p.def
}

var errNoKuscia = errors.New("kuscia client not configured")

func (p *poolKuscia) GenerateKeyCerts(ctx context.Context, nodeID string) (*service.KeyCerts, error) {
	if p.certs == nil {
		return nil, errNoKuscia
	}
	return p.certs.GenerateKeyCerts(ctx, nodeID, "vote", 0)
}

func (p *poolKuscia) DomainCert(ctx context.Context, nodeID, channel string) string {
	c := p.client(channel)
	if c == nil {
		return ""
	}
	ds, err := c.BatchQueryDomain(ctx, []string{nodeID})
	if err != nil || len(ds) == 0 {
		return ""
	}
	return ds[0].Cert
}

func (p *poolKuscia) CreatePartnerDomain(ctx context.Context, channel, domainID, cert string) error {
	c := p.client(channel)
	if c == nil {
		return errNoKuscia
	}
	return c.CreateDomain(ctx, &kuscia.CreateDomainRequest{
		DomainID: domainID,
		Role:     kuscia.DomainRolePartner,
		Cert:     cert,
		AuthCenter: &kuscia.AuthCenter{
			AuthenticationType: kuscia.AuthTypeToken,
			TokenGenMethod:     kuscia.TokenGenRSAGen,
		},
	})
}

func (p *poolKuscia) DeleteDomain(ctx context.Context, channel, domainID string) error {
	c := p.client(channel)
	if c == nil {
		return errNoKuscia
	}
	return c.DeleteDomain(ctx, domainID)
}

func (p *poolKuscia) DomainRouteExists(ctx context.Context, channel, src, dst string) (bool, error) {
	c := p.client(channel)
	if c == nil {
		return false, errNoKuscia
	}
	return c.DomainRouteExists(ctx, src, dst)
}

func (p *poolKuscia) DomainRouteReady(ctx context.Context, channel, src, dst string) bool {
	c := p.client(channel)
	if c == nil {
		return false
	}
	resp, err := c.QueryDomainRoute(ctx, src, dst)
	if err != nil || resp == nil {
		return false
	}
	return resp.Data.Status.Status == "Succeeded"
}

func (p *poolKuscia) DeleteDomainRoute(ctx context.Context, channel, src, dst string) error {
	c := p.client(channel)
	if c == nil {
		return errNoKuscia
	}
	return c.DeleteDomainRoute(ctx, src, dst)
}

// --- error helpers ---

// VoteError is a Java-coded business error returned by this package. It is
// a *service.BizError so HTTP handlers map it with service.AsBizError.
func bizErr(code *errcode.ErrorCode, msg string) error {
	return &service.BizError{Code: code, Msg: msg}
}

func bizWrap(code *errcode.ErrorCode, err error, msg string) error {
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return &service.BizError{Code: code, Msg: msg, Err: err}
}
