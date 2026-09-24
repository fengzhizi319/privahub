package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// Sync actions (Java DbChangeAction values).
const (
	SyncActionCreate = "create"
	SyncActionUpdate = "update"
	SyncActionDelete = "remove"
)

// NormalizeSyncAction maps any spelling (CREATE/INSERT/UPDATE/DELETE/REMOVE,
// case-insensitive) to the Java DbChangeAction value; unknown -> "".
func NormalizeSyncAction(a string) string {
	switch strings.ToLower(strings.TrimSpace(a)) {
	case "create", "insert", "save":
		return SyncActionCreate
	case "update", "upsert":
		return SyncActionUpdate
	case "remove", "delete":
		return SyncActionDelete
	}
	return ""
}

// SyncData is the node-to-node sync envelope (Java SyncDataDTO). TableName is
// the Go table name ("vote_request") — Java entity class names
// (org.secretflow...VoteRequestDO) are accepted on input. Data is the entity
// (a GORM model value or its JSON form).
type SyncData struct {
	TableName      string      `json:"tableName"`
	LastUpdateTime string      `json:"lastUpdateTime,omitempty"`
	Action         string      `json:"action,omitempty"`
	Data           interface{} `json:"data,omitempty"`
}

// UnmarshalJSON accepts camelCase (Java) and snake_case keys; Data is kept as
// json.RawMessage so the consumer can decode it into the target model.
func (d *SyncData) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				var s string
				if json.Unmarshal(v, &s) == nil {
					return s
				}
			}
		}
		return ""
	}
	d.TableName = str("tableName", "table_name")
	d.LastUpdateTime = str("lastUpdateTime", "last_update_time")
	d.Action = str("action")
	if v, ok := raw["data"]; ok && string(v) != "null" {
		d.Data = v
	} else {
		d.Data = nil
	}
	return nil
}

// DataJSON returns Data as JSON bytes.
func (d *SyncData) DataJSON() ([]byte, error) {
	switch v := d.Data.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return v, nil
	case []byte:
		return v, nil
	default:
		return json.Marshal(v)
	}
}

// DbSyncSender pushes entity changes to another platform (Java DbSyncUtil +
// P2P data sync):
//   - EDGE:     nodeID "" (or the center) -> POST <gateway>/api/v1alpha1/vote_sync/create
//     with Host=server.center_platform_service, kuscia-origin-source=<local node>;
//     only the 7 vote-sync types are accepted (vote_request, vote_invite,
//     node_route, tee_node_datatable_management, project, project_node,
//     project_approval_config).
//   - AUTONOMY: nodeID = peer node or inst id -> POST <gateway>/api/v1alpha1/data/sync
//     with Host=secretpad.<peer node>.svc, one call per entity.
//
// Send is synchronous and returns the first remote error.
type DbSyncSender interface {
	Send(ctx context.Context, nodeID string, entities ...SyncData) error
}

type noopDbSyncSender struct{}

func (noopDbSyncSender) Send(context.Context, string, ...SyncData) error { return nil }

var (
	dbSyncSenderMu sync.RWMutex
	dbSyncSender   DbSyncSender
)

// SetDbSyncSender installs the process-wide sender (done once in wire).
func SetDbSyncSender(s DbSyncSender) {
	dbSyncSenderMu.Lock()
	dbSyncSender = s
	dbSyncSenderMu.Unlock()
}

// CurrentDbSyncSender returns the installed sender, or a no-op sender when
// none is wired (unit tests, CENTER). Never nil.
func CurrentDbSyncSender() DbSyncSender {
	dbSyncSenderMu.RLock()
	s := dbSyncSender
	dbSyncSenderMu.RUnlock()
	if s == nil {
		return noopDbSyncSender{}
	}
	return s
}

type syncConsumerKey struct{}

// WithSyncConsumer marks ctx as a sync consumer write (Java
// DataSyncConsumerContext): GORM changes made with it are not re-published by
// the data-sync producer.
func WithSyncConsumer(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, syncConsumerKey{}, true)
}

// IsSyncConsumer reports whether ctx was marked by WithSyncConsumer.
func IsSyncConsumer(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(syncConsumerKey{}).(bool)
	return v
}
