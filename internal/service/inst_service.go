package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	pcrypto "github.com/fengzhizi319/privahub/pkg/crypto"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Inst constants (Java InstConstants / NodeInstTokenStateEnum).
const (
	InstMaxNodeNum      = 10
	InstTokenUnused     = "UNUSED"
	InstTokenUsed       = "USED"
	instTokenExpire     = 30 * time.Minute // Java TokenUtil default
	certFileMaxSize     = 1 << 20          // Java FileUtils.CERT_FILE_MAX_SIZE order of magnitude
	instRegisterModeP2P = "p2p"
)

var domainIDPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,61}[a-z0-9])?$`)

// InstService implements Java InstServiceImpl.
type InstService struct {
	db       *gorm.DB
	nodes    *NodeService
	certsDir string
	// defaultCAFile is used for tls/mtls registrations without an uploaded CA.
	defaultCAFile string
	createLock    sync.Mutex
	log           *zap.Logger
	// requireTLS refuses notls KusciaAPI registrations (production).
	requireTLS bool
}

// WithRequireTLS refuses inst node registrations whose KusciaAPI protocol is
// notls (production: kuscia traffic must be tls or mtls).
func (s *InstService) WithRequireTLS(v bool) *InstService {
	s.requireTLS = v
	return s
}

// NewInstService creates an InstService. certsDir is where register certs are
// stored (Java secretpad.certs.dir-path).
func NewInstService(db *gorm.DB, nodes *NodeService, certsDir, defaultCAFile string, log *zap.Logger) *InstService {
	if log == nil {
		log = zap.NewNop()
	}
	if certsDir == "" {
		certsDir = "./config/certs/"
	}
	return &InstService{db: db, nodes: nodes, certsDir: certsDir, defaultCAFile: defaultCAFile, log: log}
}

// --- DTOs ---

// InstVO is the Java InstVO.
type InstVO struct {
	InstID      string `json:"instId"`
	InstName    string `json:"instName"`
	LocalNodeID string `json:"localNodeId,omitempty"`
}

// InstTokenVO is the Java InstTokenVO.
type InstTokenVO struct {
	NodeID         string `json:"nodeId"`
	NodeName       string `json:"nodeName,omitempty"`
	InstToken      string `json:"instToken"`
	CreateTime     string `json:"createTime,omitempty"`
	InstTokenState string `json:"instTokenState,omitempty"`
}

// InstCreateNodeRequest is the inst/node/add request (Java CreateNodeRequest).
type InstCreateNodeRequest struct {
	Name string `json:"name"`
	Mode int    `json:"mode"`
}

// InstRegisterRequest is the json_data of inst/node/register (Java InstRegisterRequest).
type InstRegisterRequest struct {
	InstToken string `json:"token"`
	NodeID    string `json:"domainId"`
	Host      string `json:"host"`
	// Port is the node's KusciaAPI gRPC port (Java). HTTPPort is the KusciaAPI
	// HTTP port used by this Go backend; when absent, 8083 maps to 8082 and any
	// other value is used as-is.
	Port      int    `json:"port"`
	HTTPPort  int    `json:"httpPort"`
	TransPort int    `json:"transPort"`
	Mode      string `json:"mode"`
	Protocol  string `json:"protocol"`

	CertFile []byte `json:"-"`
	KeyFile  []byte `json:"-"`
	Token    []byte `json:"-"` // token file content (KusciaAPI token)
	CAFile   []byte `json:"-"` // optional CA (Go extension)
}

// --- helpers ---

func (s *InstService) ownerID(ctx context.Context) (string, error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok || u == nil || u.OwnerID == "" {
		return "", bizErr(errcode.AuthFailed, "no login user")
	}
	return u.OwnerID, nil
}

func (s *InstService) findInstNode(ctx context.Context, instID, nodeID string) (*model.NodeDO, error) {
	var n model.NodeDO
	if err := s.db.WithContext(ctx).Where("inst_id = ? AND node_id = ?", instID, nodeID).First(&n).Error; err != nil {
		return nil, ErrNodeNotFound
	}
	return &n, nil
}

// --- API ---

// GetInst returns the login user's institution (Java getInst): instId must
// equal the login ownerId.
func (s *InstService) GetInst(ctx context.Context, instID string) (*InstVO, error) {
	owner, err := s.ownerID(ctx)
	if err != nil {
		return nil, err
	}
	if instID == "" {
		instID = owner
	}
	if !strings.EqualFold(owner, instID) {
		return nil, bizErr(errcode.InstMismatchLogin, "owner not match")
	}
	var inst model.InstDO
	if err := s.db.WithContext(ctx).Where("inst_id = ?", instID).First(&inst).Error; err != nil {
		return nil, bizErr(errcode.InstNotExists, "")
	}
	return &InstVO{InstID: inst.InstID, InstName: inst.Name, LocalNodeID: s.nodes.localNodeID()}, nil
}

// ListNodes lists the nodes of the login institution with Kuscia status (Java listNode).
func (s *InstService) ListNodes(ctx context.Context) ([]NodeVO, error) {
	instID, err := s.ownerID(ctx)
	if err != nil {
		return nil, err
	}
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&model.InstDO{}).Where("inst_id = ?", instID).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, bizErr(errcode.InstNotExists, "")
	}
	var nodes []model.NodeDO
	if err := s.db.WithContext(ctx).Where("inst_id = ?", instID).Find(&nodes).Error; err != nil {
		return nil, err
	}
	out := make([]NodeVO, 0, len(nodes))
	for i := range nodes {
		vo, err := s.nodes.nodeVOP2P(ctx, &nodes[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *vo)
	}
	return out, nil
}

// CreateNode adds a node to the login institution and signs its inst token
// (Java createNode + NodeManager.createP2PNodeForInst). Serialized with a lock
// and limited to InstMaxNodeNum nodes per institution.
func (s *InstService) CreateNode(ctx context.Context, req *InstCreateNodeRequest) (*InstTokenVO, error) {
	instID, err := s.ownerID(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || strings.TrimSpace(req.Name) == "" {
		return nil, bizErr(errcode.ParamError, "name is required")
	}
	s.createLock.Lock()
	defer s.createLock.Unlock()

	var cnt int64
	if err := s.db.WithContext(ctx).Model(&model.NodeDO{}).Where("inst_id = ?", instID).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt >= InstMaxNodeNum {
		return nil, bizErr(errcode.InstNodeCountLimited, "")
	}
	nodeID, err := s.nodes.newUniqueNodeID(ctx, s.db)
	if err != nil {
		return nil, err
	}
	token, err := SignInstToken(instID, nodeID)
	if err != nil {
		return nil, bizWrap(errcode.NodeCreateError, err, "")
	}
	node := &model.NodeDO{
		InstID:         instID,
		ControlNodeID:  nodeID,
		NodeID:         nodeID,
		NetAddress:     nodeID + ":1080",
		Type:           nodeTypeNormal,
		Mode:           req.Mode,
		Name:           req.Name,
		MasterNodeID:   firstNonEmpty(s.nodes.localNodeID(), "master"),
		InstToken:      token,
		InstTokenState: InstTokenUnused,
	}
	if err := s.db.WithContext(ctx).Create(node).Error; err != nil {
		return nil, bizWrap(errcode.NodeCreateError, err, "")
	}
	return &InstTokenVO{
		NodeID: nodeID, NodeName: node.Name, InstToken: token,
		CreateTime: fmtTime(node.GmtCreate), InstTokenState: InstTokenUnused,
	}, nil
}

// GetToken returns the stored inst token and its state (Java getToken).
func (s *InstService) GetToken(ctx context.Context, nodeID string) (*InstTokenVO, error) {
	instID, err := s.ownerID(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.findInstNode(ctx, instID, nodeID)
	if err != nil {
		return nil, err
	}
	return &InstTokenVO{NodeID: n.NodeID, NodeName: n.Name, InstToken: n.InstToken, InstTokenState: n.InstTokenState}, nil
}

// NewToken re-signs the inst token and resets its state to UNUSED (Java newToken).
func (s *InstService) NewToken(ctx context.Context, nodeID string) (*InstTokenVO, error) {
	instID, err := s.ownerID(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.findInstNode(ctx, instID, nodeID)
	if err != nil {
		return nil, err
	}
	token, err := SignInstToken(instID, nodeID)
	if err != nil {
		return nil, bizWrap(errcode.NodeTokenError, err, "")
	}
	if err := s.db.WithContext(ctx).Model(&model.NodeDO{}).Where("id = ?", n.ID).
		Updates(map[string]interface{}{"inst_token": token, "inst_token_state": InstTokenUnused}).Error; err != nil {
		return nil, err
	}
	return &InstTokenVO{NodeID: nodeID, NodeName: n.Name, InstToken: token, InstTokenState: InstTokenUnused}, nil
}

// DeleteNode deletes an institution node (Java NodeManager.deleteNode(inst, nodeId)):
// the local node is protected; routes, project usage and reviewing approvals
// block deletion; the Kuscia domain is deleted through the node channel and the
// channel unregistered; the row is hard-deleted.
func (s *InstService) DeleteNode(ctx context.Context, nodeID string) error {
	instID, err := s.ownerID(ctx)
	if err != nil {
		return err
	}
	if strings.EqualFold(nodeID, s.nodes.localNodeID()) {
		return bizErr(errcode.NodeDeleteError, "localNode forbid delete")
	}
	n, err := s.findInstNode(ctx, instID, nodeID)
	if err != nil {
		return err
	}
	var routes int64
	if err := s.db.WithContext(ctx).Model(&model.NodeRouteDO{}).
		Where("src_node_id = ? OR dst_node_id = ?", nodeID, nodeID).Count(&routes).Error; err != nil {
		return err
	}
	if routes > 0 {
		return bizErr(errcode.NodeDeleteError, "node route is not empty")
	}
	if err := s.nodes.checkNodeNotInUse(ctx, nodeID, true); err != nil {
		return err
	}
	if err := s.deleteKusciaDomain(ctx, nodeID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Unscoped().Delete(&model.NodeDO{}, n.ID).Error
}

func (s *InstService) deleteKusciaDomain(ctx context.Context, nodeID string) error {
	pool := s.nodes.Pool()
	if pool == nil || !pool.IsRegistered(nodeID) {
		s.log.Info("no registered channel, delete success", zap.String("node_id", nodeID))
		return nil
	}
	c := pool.Get(nodeID)
	exists, err := c.DomainExists(ctx, nodeID)
	if err != nil {
		return bizWrap(errcode.NodeDeleteError, err, "")
	}
	if exists {
		if err := c.DeleteDomain(ctx, nodeID); err != nil {
			return bizWrap(errcode.NodeDeleteError, err, "kuscia delete domain failed")
		}
	}
	if err := pool.Unregister(nodeID); err != nil {
		s.log.Warn("unregister kuscia channel failed", zap.String("node_id", nodeID), zap.Error(err))
	}
	return nil
}

// RegisterNode registers a P2P node's KusciaAPI endpoint (Java registerNode):
// token must match the stored UNUSED inst token and verify as a JWT; cert/key
// are stored under certsDir/<instId>/<nodeId>/ and validated; the endpoint is
// registered in the client pool; netAddress/protocol are updated and the
// token marked USED. remoteHost is the caller address (Java uses the request's
// remote host, not json_data.host).
func (s *InstService) RegisterNode(ctx context.Context, req *InstRegisterRequest, remoteHost string) error {
	if err := validateRegisterRequest(req); err != nil {
		return err
	}
	var node model.NodeDO
	if err := s.db.WithContext(ctx).Where("node_id = ?", req.NodeID).First(&node).Error; err != nil {
		return bizErr(errcode.NodeNotExistError, "node is invalid")
	}
	instID := node.InstID
	var inst model.InstDO
	if instID == "" || s.db.WithContext(ctx).Where("inst_id = ?", instID).First(&inst).Error != nil {
		return bizErr(errcode.InstNotExists, "inst is invalid")
	}
	if node.InstTokenState == InstTokenUsed {
		return bizErr(errcode.InstTokenUsed, "instToken is used")
	}
	if node.InstToken == "" || !pcrypto.ConstantTimeEqual(req.InstToken, node.InstToken) {
		return bizErr(errcode.InstTokenMismatch, "token is expired")
	}
	if !VerifyInstToken(req.InstToken, instID, req.NodeID) {
		return bizErr(errcode.InstTokenMismatch, "instToken is invalid")
	}

	if err := validateCertAndKey(req.CertFile, req.KeyFile); err != nil {
		return bizWrap(errcode.InstRegisterCheckFailed, err, "cert is invalid")
	}
	if p := strings.ToLower(req.Protocol); s.requireTLS && p != "tls" && p != "mtls" {
		return bizErr(errcode.InstRegisterCheckFailed, "protocol must be TLS or MTLS in production")
	}

	host := firstNonEmpty(remoteHost, req.Host)
	dir := filepath.Join(s.certsDir, instID, req.NodeID)
	certPath, err := writeSecureFile(dir, "client.crt", req.CertFile)
	if err != nil {
		return err
	}
	keyPath, err := writeSecureFile(dir, "client.pem", req.KeyFile)
	if err != nil {
		return err
	}
	tokenPath, err := writeSecureFile(dir, "token", req.Token)
	if err != nil {
		return err
	}
	caPath := s.defaultCAFile
	if len(req.CAFile) > 0 {
		if caPath, err = writeSecureFile(dir, "ca.crt", req.CAFile); err != nil {
			return err
		}
	}

	protocol := strings.ToLower(req.Protocol)
	if (protocol == "tls" || protocol == "mtls") && caPath == "" {
		return bizErr(errcode.InstRegisterCheckFailed, "caFile cannot be null when protocol is TLS or MTLS")
	}
	httpPort := req.HTTPPort
	if httpPort == 0 {
		httpPort = req.Port
		if httpPort == 8083 {
			httpPort = 8082
		}
	}
	ep := kuscia.NodeEndpoint{
		DomainID: req.NodeID,
		Mode:     strings.ToLower(req.Mode),
		Persist:  true,
		ClientConfig: kuscia.ClientConfig{
			Host: host, Port: httpPort, Protocol: protocol,
			CertFile: certPath, KeyFile: keyPath, CAFile: caPath, Token: tokenPath,
		},
	}
	pool := s.nodes.Pool()
	if pool == nil {
		return bizErr(errcode.InstRegisterCheckFailed, "kuscia client pool not configured")
	}
	if err := pool.Register(ep); err != nil {
		return bizWrap(errcode.InstRegisterCheckFailed, err, "channel not exist")
	}
	if !pool.IsRegistered(req.NodeID) {
		return bizErr(errcode.InstRegisterCheckFailed, "channel not exist")
	}
	address := fmt.Sprintf("%s:%d", host, req.TransPort)
	return s.db.WithContext(ctx).Model(&model.NodeDO{}).Where("id = ?", node.ID).Updates(map[string]interface{}{
		"net_address":      address,
		"protocol":         protocol,
		"inst_token_state": InstTokenUsed,
	}).Error
}

func validateRegisterRequest(req *InstRegisterRequest) error {
	if req == nil {
		return bizErr(errcode.InstRegisterCheckFailed, "register request is invalid")
	}
	switch {
	case strings.TrimSpace(req.InstToken) == "":
		return bizErr(errcode.InstRegisterCheckFailed, "token is required")
	case !domainIDPattern.MatchString(req.NodeID):
		return bizErr(errcode.InstRegisterCheckFailed, "domainId must be a valid domainId")
	case req.Port < 0 || req.Port > 65535 || req.TransPort < 0 || req.TransPort > 65535 || req.HTTPPort < 0 || req.HTTPPort > 65535:
		return bizErr(errcode.InstRegisterCheckFailed, "invalid port")
	case !strings.EqualFold(req.Mode, instRegisterModeP2P):
		return bizErr(errcode.InstRegisterCheckFailed, "Invalid mode, only p2p supported")
	}
	switch strings.ToLower(req.Protocol) {
	case "tls", "mtls", "notls":
	default:
		return bizErr(errcode.InstRegisterCheckFailed, "protocol must be TLS or MTLS or NOTLS")
	}
	for name, f := range map[string][]byte{"certFile": req.CertFile, "keyFile": req.KeyFile, "token": req.Token} {
		if f == nil {
			return bizErr(errcode.InstRegisterCheckFailed, name+" is required")
		}
		if len(f) > certFileMaxSize {
			return bizErr(errcode.InstRegisterCheckFailed, name+" is too large")
		}
	}
	return nil
}

func writeSecureFile(dir, name string, content []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", bizWrap(errcode.InstFileOperationFailed, err, "")
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o600); err != nil {
		return "", bizWrap(errcode.InstFileOperationFailed, err, "")
	}
	return p, nil
}

// validateCertAndKey mirrors Java CertUtils.loadX509Cert / loadPrivateKey:
// non-blank content must parse as an X.509 certificate / private key.
func validateCertAndKey(certPEM, keyPEM []byte) error {
	if strings.TrimSpace(string(certPEM)) != "" {
		block, _ := pem.Decode(certPEM)
		if block == nil {
			return errors.New("cert is not PEM")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return fmt.Errorf("parse cert: %w", err)
		}
	}
	if strings.TrimSpace(string(keyPEM)) != "" {
		block, _ := pem.Decode(keyPEM)
		if block == nil {
			return errors.New("key is not PEM")
		}
		if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
			if _, err2 := x509.ParsePKCS1PrivateKey(block.Bytes); err2 != nil {
				if _, err3 := x509.ParseECPrivateKey(block.Bytes); err3 != nil {
					return fmt.Errorf("parse key: %w", err)
				}
			}
		}
	}
	return nil
}
