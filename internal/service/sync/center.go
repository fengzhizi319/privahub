package datasync

import (
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/service"
)

// SseSender is the part of service.SseServer used by the center dispatcher.
type SseSender interface {
	SendE(nodeID string, data *service.SyncData) error
	IsSubscribed(nodeID, tableName string) bool
	NodeIDs() []string
}

// CenterDispatcher pushes captured changes to connected edges (Java
// CenterDataSyncProducerTemplate + CenterDataSyncDataBufferTemplate +
// DbChangeEventListener, 3s period) and serves the incremental catch-up of
// a newly connected edge (JpaSyncDataService.syncByLastUpdateTime).
type CenterDispatcher struct {
	db         *gorm.DB
	reg        *Registry
	sse        SseSender
	tables     TableSet
	log        *zap.Logger
	interval   time.Duration
	maxRetries int

	mu    sync.Mutex
	queue []*ChangeEvent

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewCenterDispatcher creates the dispatcher.
func NewCenterDispatcher(db *gorm.DB, reg *Registry, sse SseSender, tables TableSet, interval time.Duration, maxRetries int, log *zap.Logger) *CenterDispatcher {
	if reg == nil {
		reg = DefaultRegistry()
	}
	if log == nil {
		log = zap.NewNop()
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &CenterDispatcher{db: db, reg: reg, sse: sse, tables: tables, log: log,
		interval: interval, maxRetries: maxRetries, stopCh: make(chan struct{})}
}

// filtered mirrors CenterDataSyncProducerTemplate.filter: only configured
// tables; jobs and tasks only on update.
func (d *CenterDispatcher) filtered(ev *ChangeEvent) bool {
	if !d.tables.Has(ev.Table) {
		return true
	}
	if ev.Table == TableProjectJob || ev.Table == TableProjectJobTask {
		return ev.Action != service.SyncActionUpdate
	}
	return false
}

// Push enqueues a change (the producer sink).
func (d *CenterDispatcher) Push(ev *ChangeEvent) {
	if ev == nil || d.filtered(ev) {
		return
	}
	d.mu.Lock()
	d.queue = append(d.queue, ev)
	d.mu.Unlock()
}

// Pending returns the queue length.
func (d *CenterDispatcher) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.queue)
}

// Start runs the dispatch loop.
func (d *CenterDispatcher) Start() {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		t := time.NewTicker(d.interval)
		defer t.Stop()
		for {
			select {
			case <-d.stopCh:
				d.Flush()
				return
			case <-t.C:
				d.Flush()
			}
		}
	}()
}

// Stop stops the loop after a final flush.
func (d *CenterDispatcher) Stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
	d.wg.Wait()
}

// Flush delivers every queued change once; failed deliveries are re-queued
// up to maxRetries times.
func (d *CenterDispatcher) Flush() {
	d.mu.Lock()
	batch := d.queue
	d.queue = nil
	d.mu.Unlock()
	var retry []*ChangeEvent
	for _, ev := range batch {
		for _, failed := range d.deliver(ev) {
			if failed.Retries < d.maxRetries {
				retry = append(retry, failed)
			} else {
				d.log.Error("data sync drop change after retries", zap.String("table", failed.Table), zap.String("node", failed.DstNode))
			}
		}
	}
	if len(retry) > 0 {
		d.mu.Lock()
		d.queue = append(retry, d.queue...)
		d.mu.Unlock()
	}
}

// targets returns the nodes an event goes to.
func (d *CenterDispatcher) targets(ev *ChangeEvent) []string {
	if ev.DstNode != "" {
		return []string{ev.DstNode}
	}
	if ev.Route.Broadcast {
		return d.sse.NodeIDs()
	}
	ids := ev.Route.NodeIDs
	if len(ids) == 0 {
		ids = projectNodeIDs(d.db, ev.Route.ProjectID)
	}
	return ids
}

func (d *CenterDispatcher) deliver(ev *ChangeEvent) []*ChangeEvent {
	var failed []*ChangeEvent
	msg := ev.SyncData()
	for _, n := range d.targets(ev) {
		if !ev.Route.Broadcast && !d.sse.IsSubscribed(n, ev.Table) {
			continue
		}
		if err := d.sse.SendE(n, msg); err != nil {
			d.log.Warn("data sync push failed, will retry", zap.String("node", n), zap.String("table", ev.Table), zap.Error(err))
			cp := *ev
			cp.DstNode = n
			cp.Retries++
			failed = append(failed, &cp)
		}
	}
	return failed
}

// InitialSync sends every row of the subscribed tables modified after the
// client's lastUpdateTime ("0" = all) and visible to nodeID (Java
// syncByLastUpdateTime + SseSendFlag). It is the SseServer open hook.
func (d *CenterDispatcher) InitialSync(nodeID string, subs []service.SyncData) {
	for _, sub := range subs {
		table, ok := d.reg.Canonical(sub.TableName)
		if !ok || !d.tables.Has(table) {
			continue
		}
		rows, err := d.rowsSince(table, sub.LastUpdateTime)
		if err != nil {
			d.log.Warn("data sync initial query failed", zap.String("table", table), zap.Error(err))
			continue
		}
		d.log.Info("data sync start", zap.String("node", nodeID), zap.String("table", table),
			zap.String("last_update_time", sub.LastUpdateTime), zap.Int("rows", len(rows)))
		for _, r := range rows {
			if !Visible(d.db, RouteOf(table, r), nodeID) {
				continue
			}
			msg := &service.SyncData{TableName: table, Action: service.SyncActionCreate, Data: r, LastUpdateTime: sub.LastUpdateTime}
			if err := d.sse.SendE(nodeID, msg); err != nil {
				d.log.Warn("data sync initial push failed", zap.String("node", nodeID), zap.Error(err))
				return
			}
		}
	}
}

func (d *CenterDispatcher) rowsSince(table, lastUpdateTime string) ([]interface{}, error) {
	info, _ := d.reg.info(table)
	slice := reflect.New(reflect.SliceOf(reflect.PointerTo(info.typ)))
	q := d.db.Model(reflect.New(info.typ).Interface())
	if lastUpdateTime != "" && lastUpdateTime != "0" {
		if t, ok := ParseTime(lastUpdateTime); ok {
			if col := info.timeColumn(); col != "" {
				q = q.Where(col+" > ?", t)
			}
		}
	}
	if err := q.Find(slice.Interface()).Error; err != nil {
		return nil, err
	}
	out := make([]interface{}, 0, slice.Elem().Len())
	for i := 0; i < slice.Elem().Len(); i++ {
		out = append(out, slice.Elem().Index(i).Interface())
	}
	return out, nil
}
