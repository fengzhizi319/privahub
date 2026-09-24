package datasync

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
)

// checkpointLayout is the edge_data_sync_log time format (Java yyyy-MM-dd HH:mm:ss).
const checkpointLayout = "2006-01-02 15:04:05"

// EdgeClientOptions configures the EDGE SSE client.
type EdgeClientOptions struct {
	// Gateway is the local Kuscia gateway (host:port or URL), Java secretpad.gateway.
	Gateway string
	// CenterHost is the Host header routed by the gateway to the center
	// (Java secretpad.center-platform-service).
	CenterHost string
	// NodeID is this edge node (kuscia-origin-source).
	NodeID string
	// Tables are the subscribed tables.
	Tables TableSet
	// Reconnect is the reconnect period (default 5s).
	Reconnect time.Duration
	// IdleTimeout drops a silent stream (default 60s).
	IdleTimeout time.Duration
	// HTTPClient overrides the streaming client (tests).
	HTTPClient *http.Client
}

// EdgeClient is the EDGE side of the center->edge sync (Java EdgeSyncTask +
// EdgeDataSyncServiceImpl).
type EdgeClient struct {
	db       *gorm.DB
	reg      *Registry
	consumer *Consumer
	opts     EdgeClientOptions
	log      *zap.Logger
	client   *http.Client

	connected atomic.Bool
	received  atomic.Int64

	stopOnce sync.Once
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewEdgeClient creates the EDGE SSE client.
func NewEdgeClient(db *gorm.DB, reg *Registry, consumer *Consumer, opts EdgeClientOptions, log *zap.Logger) *EdgeClient {
	if reg == nil {
		reg = DefaultRegistry()
	}
	if log == nil {
		log = zap.NewNop()
	}
	if opts.Reconnect <= 0 {
		opts.Reconnect = 5 * time.Second
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = 60 * time.Second
	}
	c := opts.HTTPClient
	if c == nil {
		// No overall timeout: the stream is long-lived; idle is enforced by the reader.
		c = &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	}
	return &EdgeClient{db: db, reg: reg, consumer: consumer, opts: opts, log: log, client: c}
}

// Connected reports whether the SSE stream is currently open (Java sseSate).
func (e *EdgeClient) Connected() bool { return e.connected.Load() }

// Received returns the number of applied data events.
func (e *EdgeClient) Received() int64 { return e.received.Load() }

// Start runs the connect/reconnect loop (every opts.Reconnect when disconnected).
func (e *EdgeClient) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			if err := e.RunOnce(ctx); err != nil && ctx.Err() == nil {
				e.log.Warn("edge data sync stream ended", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(e.opts.Reconnect):
			}
		}
	}()
}

// Stop stops the client.
func (e *EdgeClient) Stop() {
	e.stopOnce.Do(func() {
		if e.cancel != nil {
			e.cancel()
		}
	})
	e.wg.Wait()
	e.connected.Store(false)
}

func gatewayURL(gw string) string {
	gw = strings.TrimRight(strings.TrimSpace(gw), "/")
	if !strings.HasPrefix(gw, "http://") && !strings.HasPrefix(gw, "https://") {
		gw = "http://" + gw
	}
	return gw
}

// Checkpoints computes the subscription list (Java EdgeDataSyncServiceImpl.log
// + logTableLastUpdateTime): per table the latest local modification time
// ("0" for node and empty tables), persisted into edge_data_sync_log.
func (e *EdgeClient) Checkpoints(ctx context.Context) []service.SyncData {
	out := make([]service.SyncData, 0, len(e.opts.Tables))
	for _, table := range e.opts.Tables.List() {
		lut := "0"
		if table != TableNode {
			lut = e.lastUpdateTime(ctx, table)
		}
		rec := &model.EdgeDataSyncLogDO{SyncTableName: table, LastUpdateTime: lut}
		if err := e.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "table_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_update_time"}),
		}).Create(rec).Error; err != nil {
			e.log.Debug("save edge_data_sync_log failed", zap.String("table", table), zap.Error(err))
		}
		out = append(out, service.SyncData{TableName: table, LastUpdateTime: lut})
	}
	return out
}

func (e *EdgeClient) lastUpdateTime(ctx context.Context, table string) string {
	info, ok := e.reg.info(table)
	if !ok {
		return "0"
	}
	col := info.timeColumn()
	if col == "" {
		return "0"
	}
	row := reflect.New(info.typ)
	res := e.db.WithContext(ctx).Unscoped().Model(row.Interface()).Order(col + " DESC").Limit(1).Find(row.Interface())
	if res.Error != nil || res.RowsAffected == 0 {
		return "0"
	}
	name := "GmtModified"
	if col == "gmt_create" {
		name = "GmtCreate"
	}
	f := row.Elem().FieldByName(name)
	if !f.IsValid() || f.Type() != timeType {
		return "0"
	}
	t := f.Interface().(time.Time)
	if t.IsZero() {
		return "0"
	}
	return t.In(time.Local).Format(checkpointLayout)
}

// streamURL builds <gateway>/sync?p=<json subscriptions>.
func (e *EdgeClient) streamURL(subs []service.SyncData) (string, error) {
	b, err := json.Marshal(subs)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("p", string(b))
	return gatewayURL(e.opts.Gateway) + "/sync?" + q.Encode(), nil
}

// RunOnce opens one SSE stream and consumes it until it ends.
func (e *EdgeClient) RunOnce(ctx context.Context) error {
	subs := e.Checkpoints(ctx)
	u, err := e.streamURL(subs)
	if err != nil {
		return err
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(sctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if e.opts.CenterHost != "" {
		req.Host = e.opts.CenterHost
	}
	req.Header.Set("kuscia-origin-source", e.opts.NodeID)
	req.Header.Set("Accept", "text/event-stream")
	innerauth.Apply(req)
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("sse connect failed: http %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	e.connected.Store(true)
	defer e.connected.Store(false)
	e.log.Info("edge data sync connected", zap.String("gateway", e.opts.Gateway), zap.Int("tables", len(subs)))

	// Idle watchdog: any line (including pings) resets it.
	activity := make(chan struct{}, 1)
	go func() {
		t := time.NewTimer(e.opts.IdleTimeout)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				return
			case <-activity:
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				t.Reset(e.opts.IdleTimeout)
			case <-t.C:
				e.log.Warn("edge data sync stream idle, reconnecting")
				cancel()
				return
			}
		}
	}()
	return e.consume(sctx, resp.Body, activity)
}

// consume parses the SSE stream (id:/data: fields, blank-line dispatch).
func (e *EdgeClient) consume(ctx context.Context, r io.Reader, activity chan<- struct{}) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
	var id string
	var data strings.Builder
	for sc.Scan() {
		select {
		case activity <- struct{}{}:
		default:
		}
		line := sc.Text()
		if line == "" {
			if data.Len() > 0 {
				e.HandleEvent(ctx, id, data.String())
			}
			id = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			id = value
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// HandleEvent applies one SSE data event (pings are ignored).
func (e *EdgeClient) HandleEvent(ctx context.Context, id, data string) {
	if id == service.SsePingMsg || data == service.SsePingMsg {
		return
	}
	var d service.SyncData
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		e.log.Warn("sse onEvent decode error", zap.String("id", id), zap.Error(err))
		return
	}
	if d.TableName == "" {
		d.TableName = id
	}
	if _, err := e.consumer.Apply(ctx, ModeEdge, &d); err != nil {
		e.log.Warn("sse onEvent sync error", zap.String("table", d.TableName), zap.Error(err))
		return
	}
	e.received.Add(1)
}
