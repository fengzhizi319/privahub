package datasync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
)

// Paths of the node-to-node sync endpoints.
const (
	VoteSyncPath = "/api/v1alpha1/vote_sync/create"
	DataSyncPath = "/api/v1alpha1/data/sync"
)

// GatewaySenderOptions configures GatewaySender.
type GatewaySenderOptions struct {
	// Gateway is the local Kuscia gateway (Java secretpad.gateway).
	Gateway string
	// CenterHost is the center Host header (Java secretpad.center-platform-service).
	CenterHost string
	// LocalNodeID is this platform node (kuscia-origin-source).
	LocalNodeID string
	// Timeout bounds each call (default 10s).
	Timeout time.Duration
	// PlatformType decides the default route of Send ("EDGE" -> center).
	PlatformType service.PlatformType
	// HTTPClient overrides the client (tests).
	HTTPClient *http.Client
}

// GatewaySender implements service.DbSyncSender (Java DbSyncUtil) and
// PeerPoster (Java P2pDataSyncRestService) over the Kuscia gateway.
type GatewaySender struct {
	db   *gorm.DB
	reg  *Registry
	opts GatewaySenderOptions
	hc   *http.Client
}

// NewGatewaySender creates the sender.
func NewGatewaySender(db *gorm.DB, reg *Registry, opts GatewaySenderOptions) *GatewaySender {
	if reg == nil {
		reg = DefaultRegistry()
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: opts.Timeout}
	}
	return &GatewaySender{db: db, reg: reg, opts: opts, hc: hc}
}

// DbSyncRequest is Java DbSyncRequest {syncDataType, projectNodesInfo}.
type DbSyncRequest struct {
	SyncDataType     string          `json:"syncDataType"`
	ProjectNodesInfo json.RawMessage `json:"projectNodesInfo"`
}

// VoteSyncRequest is Java VoteSyncRequest {dbSyncRequests}.
type VoteSyncRequest struct {
	DbSyncRequests []DbSyncRequest `json:"dbSyncRequests"`
}

// UnmarshalJSON accepts camelCase and snake_case keys.
func (r *DbSyncRequest) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, k := range []string{"syncDataType", "sync_data_type"} {
		if v, ok := raw[k]; ok {
			_ = json.Unmarshal(v, &r.SyncDataType)
			break
		}
	}
	for _, k := range []string{"projectNodesInfo", "project_nodes_info"} {
		if v, ok := raw[k]; ok {
			r.ProjectNodesInfo = v
			break
		}
	}
	return nil
}

// UnmarshalJSON accepts camelCase and snake_case keys.
func (r *VoteSyncRequest) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, k := range []string{"dbSyncRequests", "db_sync_requests"} {
		if v, ok := raw[k]; ok {
			return json.Unmarshal(v, &r.DbSyncRequests)
		}
	}
	return nil
}

// voteSyncTypes maps Java VoteSyncTypeEnum to Go tables.
var voteSyncTypes = map[string]string{
	"VOTE_REQUEST":                  TableVoteRequest,
	"VOTE_INVITE":                   TableVoteInvite,
	"NODE_ROUTE":                    TableNodeRoute,
	"TEE_NODE_DATATABLE_MANAGEMENT": TableTeeNodeDatatableManagement,
	"PROJECT":                       TableProject,
	"PROJECT_NODE":                  TableProjectNode,
	"PROJECT_APPROVAL_CONFIG":       TableProjectApprovalConfig,
	// Go extension (not in Java DataSyncConstants): used by the Go vote
	// handlers to push project_inst rows with the project.
	"PROJECT_INST": TableProjectInst,
}

// VoteSyncTableOf returns the table of a Java syncDataType.
func VoteSyncTableOf(syncDataType string) (string, bool) {
	t, ok := voteSyncTypes[strings.ToUpper(strings.TrimSpace(syncDataType))]
	return t, ok
}

// VoteSyncTypeOf returns the Java syncDataType of a table.
func VoteSyncTypeOf(table string) (string, bool) {
	for k, v := range voteSyncTypes {
		if v == table {
			return k, true
		}
	}
	return "", false
}

type remoteResp struct {
	Status struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	} `json:"status"`
}

func (s *GatewaySender) post(ctx context.Context, path, host, origin string, body interface{}) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL(s.opts.Gateway)+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if host != "" {
		req.Host = host
	}
	if origin != "" {
		req.Header.Set("kuscia-origin-source", origin)
	}
	innerauth.Apply(req)
	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("remote call %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote call %s: http %d %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var r remoteResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("remote call %s: invalid response: %w", path, err)
	}
	if r.Status.Code != 0 {
		return fmt.Errorf("remote call %s: [%d] %s", path, r.Status.Code, r.Status.Msg)
	}
	return nil
}

// BuildVoteSyncRequest converts entities to the Java VoteSyncRequest.
func (s *GatewaySender) BuildVoteSyncRequest(entities ...service.SyncData) (*VoteSyncRequest, error) {
	req := &VoteSyncRequest{}
	for i := range entities {
		table, ok := s.reg.Canonical(entities[i].TableName)
		if !ok {
			return nil, fmt.Errorf("unknown table %q", entities[i].TableName)
		}
		typ, ok := VoteSyncTypeOf(table)
		if !ok {
			return nil, fmt.Errorf("table %q cannot be pushed through vote_sync", table)
		}
		raw, err := entities[i].DataJSON()
		if err != nil {
			return nil, err
		}
		req.DbSyncRequests = append(req.DbSyncRequests, DbSyncRequest{SyncDataType: typ, ProjectNodesInfo: raw})
	}
	return req, nil
}

// SendToCenter posts entities to the center vote_sync endpoint (Java
// DbSyncUtil.dbDataSyncToCenter).
func (s *GatewaySender) SendToCenter(ctx context.Context, entities ...service.SyncData) error {
	req, err := s.BuildVoteSyncRequest(entities...)
	if err != nil {
		return err
	}
	if len(req.DbSyncRequests) == 0 {
		return nil
	}
	return s.post(ctx, VoteSyncPath, s.opts.CenterHost, s.opts.LocalNodeID, req)
}

// RouteNodeOf resolves a peer id (node id or inst id) to the node id used in
// the gateway Host (Java P2pPaddingNodeServiceImpl.turnInstToRouteId).
func (s *GatewaySender) RouteNodeOf(dst string) string {
	if s.db == nil || dst == "" {
		return dst
	}
	var n int64
	s.db.Model(&model.NodeDO{}).Where("node_id = ?", dst).Count(&n)
	if n > 0 {
		return dst
	}
	var nodes []model.NodeDO
	s.db.Where("inst_id = ?", dst).Order("id").Find(&nodes)
	for _, nd := range nodes {
		if nd.MasterNodeID != "" && nd.MasterNodeID != "master" {
			return nd.MasterNodeID
		}
	}
	if len(nodes) > 0 {
		return nodes[0].NodeID
	}
	return dst
}

// PostDataSync posts one change to a peer (Java P2pDataSyncRestService.sync):
// Host secretpad.<route node>.svc.
func (s *GatewaySender) PostDataSync(ctx context.Context, dst string, d *service.SyncData) error {
	route := s.RouteNodeOf(dst)
	return s.post(ctx, DataSyncPath, "secretpad."+route+".svc", s.opts.LocalNodeID, d)
}

// Send implements service.DbSyncSender: EDGE (or nodeID "") -> center
// vote_sync; otherwise one data/sync call per entity to the peer.
func (s *GatewaySender) Send(ctx context.Context, nodeID string, entities ...service.SyncData) error {
	if len(entities) == 0 {
		return nil
	}
	if nodeID == "" || s.opts.PlatformType == service.PlatformEdge {
		return s.SendToCenter(ctx, entities...)
	}
	for i := range entities {
		d := entities[i]
		if table, ok := s.reg.Canonical(d.TableName); ok {
			d.TableName = table
		}
		if d.Action == "" {
			d.Action = service.SyncActionUpdate
		}
		if err := s.PostDataSync(ctx, nodeID, &d); err != nil {
			return err
		}
	}
	return nil
}
