// Package bootstrap runs the platform startup initializers of Java SecretPad
// (web/init/*): EdgeResourceInit, P2pDataInit, MasterRouteInit and
// TeeResourceInit. DbDataInit (admin seed) lives in
// internal/dao/migrations/auth_seed.go and DynamicBeanRegisterInit is the
// sync.disabled_routes middleware (wire). Every step is idempotent and
// failure-tolerant: errors are logged and the next step runs.
package bootstrap

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// DomainAPI is the KusciaAPI domain subset used by the initializers.
type DomainAPI interface {
	QueryDomain(ctx context.Context, domainID string) (*kuscia.QueryDomainResponse, error)
	CreateDomain(ctx context.Context, req *kuscia.CreateDomainRequest) error
}

// EndpointRegistrar registers a KusciaAPI endpoint (kuscia.ClientPool).
type EndpointRegistrar interface {
	Register(ep kuscia.NodeEndpoint) error
}

// DataProxyUpdater is the DataProxyService subset used here.
type DataProxyUpdater interface {
	UpdateDomain(ctx context.Context, domainID string)
	UpdateDataSourceUseDataProxyInP2p(ctx context.Context, instID string)
}

// TeeOptions mirrors TeeResourceInit properties.
type TeeOptions struct {
	Enabled  bool
	Address  string // KUSCIA_API_LITE_TEE_ADDRESS (default root-kuscia-lite-tee)
	Port     int    // KusciaAPI HTTP port of the TEE lite
	Protocol string // KUSCIA_PROTOCOL (default tls)
	CertDir  string // default config/certs/tee
}

// Deps are the initializer dependencies (nil Kuscia/Pool/DataProxy skip the
// corresponding remote actions).
type Deps struct {
	DB        *gorm.DB
	Env       *service.EnvService
	Kuscia    DomainAPI
	Pool      EndpointRegistrar
	DataProxy DataProxyUpdater
	// AdminName is the initial admin (auth.admin_name, default admin).
	AdminName string
	// InstName is the P2P institution name (Java secretpad.inst-name).
	InstName string
	// KusciaProtocol is kuscia.protocol (route http/https prefix, node protocol).
	KusciaProtocol string
	Tee            TeeOptions
	// Timeout bounds each remote call (default 15s).
	Timeout time.Duration
	Log     *zap.Logger
}

// Initializer runs the startup steps.
type Initializer struct {
	d Deps
}

// New creates an Initializer.
func New(d Deps) *Initializer {
	if d.Log == nil {
		d.Log = zap.NewNop()
	}
	if d.Env == nil {
		d.Env = service.GlobalEnv()
	}
	if d.AdminName == "" {
		d.AdminName = "admin"
	}
	if d.InstName == "" {
		d.InstName = "DEFAULT_INST"
	}
	if d.Timeout <= 0 {
		d.Timeout = 15 * time.Second
	}
	if d.Tee.Address == "" {
		d.Tee.Address = "root-kuscia-lite-tee"
	}
	if d.Tee.Protocol == "" {
		d.Tee.Protocol = "tls"
	}
	if d.Tee.CertDir == "" {
		d.Tee.CertDir = "config/certs/tee"
	}
	return &Initializer{d: d}
}

type step struct {
	name   string
	remote bool // needs Kuscia; run in background by Start
	fn     func(ctx context.Context) error
}

func (i *Initializer) steps() []step {
	env := i.d.Env
	var out []step
	switch env.GetPlatformType() {
	case service.PlatformAutonomy:
		out = append(out, step{"P2pDataInit", false, i.P2pDataInit})
		out = append(out, step{"P2pDataProxy", true, i.p2pDataProxy})
	case service.PlatformEdge:
		out = append(out, step{"EdgeResourceInit", true, i.EdgeResourceInit})
	case service.PlatformCenter, service.PlatformTest:
		out = append(out, step{"MasterRouteInit", false, i.MasterRouteInit})
		out = append(out, step{"TeeResourceInit", false, i.TeeResourceInitDB})
		out = append(out, step{"TeeResourceInitKuscia", true, i.TeeResourceInitKuscia})
	}
	return out
}

func (i *Initializer) run(ctx context.Context, s step) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
		if err != nil {
			i.d.Log.Error("startup init failed", zap.String("step", s.name), zap.Error(err))
		} else {
			i.d.Log.Info("startup init done", zap.String("step", s.name))
		}
	}()
	return s.fn(ctx)
}

// Run executes every step synchronously and returns the joined errors.
func (i *Initializer) Run(ctx context.Context) error {
	var errs []error
	for _, s := range i.steps() {
		if err := i.run(ctx, s); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.name, err))
		}
	}
	return errors.Join(errs...)
}

// Start runs the local (DB) steps now and the Kuscia steps in the
// background; the returned channel is closed when everything finished.
func (i *Initializer) Start(ctx context.Context) <-chan struct{} {
	var remote []step
	for _, s := range i.steps() {
		if s.remote {
			remote = append(remote, s)
			continue
		}
		_ = i.run(ctx, s)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, s := range remote {
			_ = i.run(ctx, s)
		}
	}()
	return done
}

func (i *Initializer) timeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, i.d.Timeout)
}

// ---------------------------------------------------------------------------
// EdgeResourceInit
// ---------------------------------------------------------------------------

// EdgeResourceInit syncs the local node from Kuscia (Java EdgeResourceInit):
// the node of the first account's owner is (re)written from QueryDomain and
// DataProxy is enabled for it.
func (i *Initializer) EdgeResourceInit(ctx context.Context) error {
	db := i.d.DB.WithContext(ctx)
	ownerID := i.d.Env.GetPlatformNodeId()
	var acc model.UserAccountsDO
	if db.Order("id").Limit(1).Find(&acc).RowsAffected > 0 && acc.OwnerID != "" {
		ownerID = acc.OwnerID
	}
	if i.d.Kuscia == nil {
		return errors.New("kuscia client not configured")
	}
	cctx, cancel := i.timeout(ctx)
	resp, err := i.d.Kuscia.QueryDomain(cctx, ownerID)
	cancel()
	if err != nil || resp == nil || resp.Status.Code != 0 {
		return fmt.Errorf("kuscia lite %s not ready: %v", ownerID, err)
	}
	domainID := resp.Data.DomainID
	if domainID == "" {
		domainID = ownerID
	}
	var node model.NodeDO
	found := db.Unscoped().Where("node_id = ?", domainID).Limit(1).Find(&node).RowsAffected > 0
	node.NodeID = domainID
	node.ControlNodeID = domainID
	node.Name = ""
	node.Token = ""
	node.Type = "normal"
	node.Mode = 1
	node.InstID = ""
	node.IsDeleted = 0
	node.DeletedAt = gorm.DeletedAt{}
	if found {
		err = db.Unscoped().Save(&node).Error
	} else {
		err = db.Create(&node).Error
	}
	if err != nil {
		return err
	}
	if i.d.DataProxy != nil {
		cctx, cancel := i.timeout(ctx)
		i.d.DataProxy.UpdateDomain(cctx, domainID)
		cancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// P2pDataInit
// ---------------------------------------------------------------------------

const instIDChars = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomInstID() string {
	b := make([]byte, 8)
	for k := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(instIDChars))))
		if err != nil {
			b[k] = 'x'
			continue
		}
		b[k] = instIDChars[n.Int64()]
	}
	return string(b)
}

// P2pDataInit initializes the local inst + node (Java NodeManager.initialNode)
// and moves the admin account to the inst (resumeAccountsInst). The inst id
// is published through EnvService.SetInstID.
func (i *Initializer) P2pDataInit(ctx context.Context) error {
	nodeID := i.d.Env.GetPlatformNodeId()
	if nodeID == "" {
		return errors.New("platform node id is empty")
	}
	var instID string
	err := i.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		netAddr := "127.0.0.1:28080"
		var node model.NodeDO
		found := tx.Unscoped().Where("node_id = ?", nodeID).Limit(1).Find(&node).RowsAffected > 0
		if found {
			if node.NetAddress != "" {
				netAddr = node.NetAddress
			}
			instID = node.InstID
		}
		if instID == "" {
			instID = randomInstID()
			if err := tx.Create(&model.InstDO{InstID: instID, Name: i.d.InstName}).Error; err != nil {
				return err
			}
		} else {
			var n int64
			tx.Model(&model.InstDO{}).Where("inst_id = ?", instID).Count(&n)
			if n == 0 {
				if err := tx.Create(&model.InstDO{InstID: instID, Name: i.d.InstName}).Error; err != nil {
					return err
				}
			}
		}
		node.NodeID = nodeID
		node.InstID = instID
		node.Description = nodeID
		node.Type = "primary"
		node.Mode = 1
		node.Name = nodeID
		node.NetAddress = netAddr
		node.ControlNodeID = nodeID
		node.Protocol = i.d.KusciaProtocol
		node.MasterNodeID = nodeID
		node.IsDeleted = 0
		node.DeletedAt = gorm.DeletedAt{}
		if found {
			if err := tx.Unscoped().Save(&node).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&node).Error; err != nil {
			return err
		}
		// resumeAccountsInst
		res := tx.Model(&model.UserAccountsDO{}).Where("name = ?", i.d.AdminName).
			Updates(map[string]interface{}{"owner_id": instID, "inst_id": instID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			i.d.Log.Warn("P2pDataInit: admin account not found", zap.String("name", i.d.AdminName))
		}
		return nil
	})
	if err != nil {
		return err
	}
	i.d.Env.SetInstID(instID)
	var nodes []string
	i.d.DB.WithContext(ctx).Model(&model.NodeDO{}).Where("inst_id = ?", instID).Pluck("node_id", &nodes)
	i.d.Env.SetEmbeddedNodes(nodes)
	i.d.Log.Info("P2pDataInit inst ready", zap.String("inst_id", instID), zap.Strings("nodes", nodes))
	return nil
}

func (i *Initializer) p2pDataProxy(ctx context.Context) error {
	if i.d.DataProxy == nil || i.d.Env.GetInstID() == "" {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	i.d.DataProxy.UpdateDataSourceUseDataProxyInP2p(cctx, i.d.Env.GetInstID())
	return nil
}

// ---------------------------------------------------------------------------
// MasterRouteInit
// ---------------------------------------------------------------------------

// MasterRouteInit prefixes the embedded routes 1/2/3 with http:// (notls) or
// https:// (Java MasterRouteInit).
func (i *Initializer) MasterRouteInit(ctx context.Context) error {
	var routes []model.NodeRouteDO
	if err := i.d.DB.WithContext(ctx).Where("route_id IN ?", []string{"1", "2", "3"}).Find(&routes).Error; err != nil {
		return err
	}
	if len(routes) == 0 {
		i.d.Log.Warn("node router list is empty")
		return nil
	}
	prefix := "https://"
	if strings.EqualFold(i.d.KusciaProtocol, "notls") {
		prefix = "http://"
	}
	for _, r := range routes {
		if strings.Contains(r.SrcNetAddress, "http") || strings.Contains(r.DstNetAddress, "http") {
			continue
		}
		if err := i.d.DB.WithContext(ctx).Model(&model.NodeRouteDO{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
			"src_net_address": prefix + r.SrcNetAddress,
			"dst_net_address": prefix + r.DstNetAddress,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// TeeResourceInit
// ---------------------------------------------------------------------------

func (i *Initializer) teeApplies() bool {
	if !i.d.Tee.Enabled || !i.d.Env.IsCenter() {
		return false
	}
	mode := strings.ToUpper(i.d.Env.GetDeployMode())
	return mode == "ALL-IN-ONE" || mode == "TEE"
}

// TeeRoutes are the Java TeeResourceInit routes (ids 3..6).
var TeeRoutes = []model.NodeRouteDO{
	{RouteID: "3", SrcNodeID: "alice", DstNodeID: "tee", SrcNetAddress: "127.0.0.1:28080", DstNetAddress: "127.0.0.1:48080"},
	{RouteID: "4", SrcNodeID: "tee", DstNodeID: "alice", SrcNetAddress: "127.0.0.1:48080", DstNetAddress: "127.0.0.1:28080"},
	{RouteID: "5", SrcNodeID: "bob", DstNodeID: "tee", SrcNetAddress: "127.0.0.1:38080", DstNetAddress: "127.0.0.1:48080"},
	{RouteID: "6", SrcNodeID: "tee", DstNodeID: "bob", SrcNetAddress: "127.0.0.1:48080", DstNetAddress: "127.0.0.1:38080"},
}

// TeeResourceInitDB creates the embedded tee node and its routes (CENTER with
// deploy mode ALL-IN-ONE/TEE). Existing rows are kept.
func (i *Initializer) TeeResourceInitDB(ctx context.Context) error {
	if !i.teeApplies() {
		return nil
	}
	db := i.d.DB.WithContext(ctx)
	var n int64
	db.Unscoped().Model(&model.NodeDO{}).Where("node_id = ?", "tee").Count(&n)
	if n == 0 {
		tee := &model.NodeDO{NodeID: "tee", Name: "tee", ControlNodeID: "tee", Auth: "tee", Description: "tee",
			InstID: "", Type: "embedded", NetAddress: "127.0.0.1:48080", Mode: 1, MasterNodeID: "master"}
		if err := db.Create(tee).Error; err != nil {
			return err
		}
	}
	for _, r := range TeeRoutes {
		r := r
		var c int64
		db.Unscoped().Model(&model.NodeRouteDO{}).Where("src_node_id = ? AND dst_node_id = ?", r.SrcNodeID, r.DstNodeID).Count(&c)
		if c > 0 {
			continue
		}
		if err := db.Create(&r).Error; err != nil {
			return err
		}
	}
	return nil
}

// TeeResourceInitKuscia creates the tee domain when missing and registers the
// tee KusciaAPI endpoint (dynamicKusciaChannelProvider.registerKuscia).
func (i *Initializer) TeeResourceInitKuscia(ctx context.Context) error {
	if !i.teeApplies() {
		return nil
	}
	var errs []error
	if i.d.Kuscia != nil {
		cctx, cancel := i.timeout(ctx)
		_, qerr := i.d.Kuscia.QueryDomain(cctx, "tee")
		cancel()
		if qerr != nil && kuscia.ErrorCode(qerr) != 0 {
			cctx, cancel := i.timeout(ctx)
			err := i.d.Kuscia.CreateDomain(cctx, &kuscia.CreateDomainRequest{
				DomainID:   "tee",
				AuthCenter: &kuscia.AuthCenter{AuthenticationType: "Token", TokenGenMethod: "UID-RSA-GEN"},
			})
			cancel()
			if err != nil {
				i.d.Log.Warn("tee init error", zap.Error(err))
			}
		} else if qerr != nil {
			errs = append(errs, fmt.Errorf("query tee domain: %w", qerr))
		}
	}
	if reg, ok := i.d.Pool.(interface{ IsRegistered(string) bool }); ok && reg.IsRegistered("tee") {
		i.d.Log.Info("tee kuscia endpoint already registered")
	} else if i.d.Pool != nil {
		dir := i.d.Tee.CertDir
		ep := kuscia.NodeEndpoint{
			DomainID: "tee",
			Mode:     "lite",
			ClientConfig: kuscia.ClientConfig{
				Host: i.d.Tee.Address, Port: i.d.Tee.Port, Protocol: i.d.Tee.Protocol,
				CertFile: filepath.Join(dir, "client.crt"), KeyFile: filepath.Join(dir, "client.pem"),
				CAFile: filepath.Join(dir, "ca.crt"), Token: filepath.Join(dir, "token"),
				Timeout: i.d.Timeout,
			},
		}
		if err := i.d.Pool.Register(ep); err != nil {
			errs = append(errs, fmt.Errorf("register tee kuscia endpoint: %w", err))
		}
	}
	return errors.Join(errs...)
}
