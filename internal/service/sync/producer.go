package datasync

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	"github.com/fengzhizi319/privahub/internal/service"
)

// ChangeEvent is one entity change (Java EntityChangeListener.DbChangeEvent).
type ChangeEvent struct {
	Table  string          `json:"table"`
	Action string          `json:"action"`
	Data   json.RawMessage `json:"data"`
	Route  Route           `json:"route"`
	// Entity is the snapshot decoded back into the model (not persisted).
	Entity interface{} `json:"-"`
	// Key identifies the row (table + business key) for queue de-duplication.
	Key string `json:"key"`
	// DstNode is the destination (P2P queues).
	DstNode string `json:"dstNode,omitempty"`
	Retries int    `json:"retries,omitempty"`
}

// SyncData converts the event to the wire envelope.
func (e *ChangeEvent) SyncData() *service.SyncData {
	return &service.SyncData{TableName: e.Table, Action: e.Action, Data: e.Data}
}

// ChangeSink receives the captured changes.
type ChangeSink func(ev *ChangeEvent)

// Producer registers GORM callbacks that capture create/update/delete of the
// tracked tables (Java EntityChangeListener @PostPersist/@PostUpdate/@PostRemove).
// Writes made with service.WithSyncConsumer contexts are ignored.
type Producer struct {
	reg     *Registry
	log     *zap.Logger
	mu      sync.RWMutex
	tracked TableSet
	sink    ChangeSink
}

const (
	cbPrefix       = "datasync:"
	instUpdateRows = "datasync:update_rows"
	instDeleteRows = "datasync:delete_rows"
)

// NewProducer creates a producer for the tracked tables.
func NewProducer(reg *Registry, tracked TableSet, sink ChangeSink, log *zap.Logger) *Producer {
	if reg == nil {
		reg = DefaultRegistry()
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Producer{reg: reg, tracked: tracked, sink: sink, log: log}
}

// Register installs the callbacks on db (idempotent per db: re-registering
// replaces the previous callbacks).
func (p *Producer) Register(db *gorm.DB) error {
	cb := db.Callback()
	steps := []struct {
		reg func() error
	}{
		{func() error {
			return cb.Create().After("gorm:create").Replace(cbPrefix+"after_create", p.afterCreate)
		}},
		{func() error {
			return cb.Update().Before("gorm:update").Replace(cbPrefix+"before_update", p.beforeUpdate)
		}},
		{func() error {
			return cb.Update().After("gorm:update").Replace(cbPrefix+"after_update", p.afterUpdate)
		}},
		{func() error {
			return cb.Delete().Before("gorm:delete").Replace(cbPrefix+"before_delete", p.beforeDelete)
		}},
		{func() error {
			return cb.Delete().After("gorm:delete").Replace(cbPrefix+"after_delete", p.afterDelete)
		}},
	}
	for _, s := range steps {
		if err := s.reg(); err != nil {
			return err
		}
	}
	return nil
}

func (p *Producer) table(tx *gorm.DB) (string, bool) {
	if tx.Statement == nil {
		return "", false
	}
	if service.IsSyncConsumer(tx.Statement.Context) {
		return "", false
	}
	t := statementTable(tx.Statement)
	if t == "" {
		return "", false
	}
	p.mu.RLock()
	ok := p.tracked.Has(t)
	p.mu.RUnlock()
	if !ok {
		return "", false
	}
	if _, known := p.reg.info(t); !known {
		return "", false
	}
	return t, true
}

// statementTable returns the table a statement writes to: the model schema
// table, else the db.Table("name") of a model-less write (map Updates/Create),
// so writes that bypass a model are captured too. Aliased or expression
// tables ("x AS t", subqueries) are not tracked.
func statementTable(st *gorm.Statement) string {
	if st.Schema != nil {
		return st.Schema.Table
	}
	t := strings.TrimSpace(st.Table)
	if t == "" || strings.ContainsAny(t, " ()") {
		return ""
	}
	return strings.Trim(t, "`\"")
}

// schemaOf returns the statement schema, else the registry schema of table
// (model-less db.Table writes).
func (p *Producer) schemaOf(tx *gorm.DB, table string) *schema.Schema {
	if tx.Statement.Schema != nil {
		return tx.Statement.Schema
	}
	if info, ok := p.reg.info(table); ok {
		return info.sch
	}
	return nil
}

func (p *Producer) emit(table, action string, entity interface{}) {
	p.mu.RLock()
	sink := p.sink
	p.mu.RUnlock()
	if sink == nil {
		return
	}
	b, err := json.Marshal(entity)
	if err != nil {
		p.log.Warn("datasync: marshal change failed", zap.String("table", table), zap.Error(err))
		return
	}
	// Snapshot: decode into a fresh model so later mutations of the caller's
	// struct do not leak into the event.
	snap, err := p.reg.Decode(table, b)
	if err != nil {
		snap = entity
	}
	sink(&ChangeEvent{
		Table:  table,
		Action: action,
		Data:   b,
		Route:  RouteOf(table, snap),
		Entity: snap,
		Key:    p.key(table, snap),
	})
}

// key builds "table|k1=v1|k2=v2" from the business key.
func (p *Producer) key(table string, entity interface{}) string {
	info, ok := p.reg.info(table)
	if !ok {
		return table
	}
	return entityKey(info, entity)
}

func entityKey(info *entityInfo, entity interface{}) string {
	v := reflect.ValueOf(entity)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return info.table
		}
		v = v.Elem()
	}
	b, _ := json.Marshal(entity)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	k := info.table
	for _, col := range info.keyCols {
		b, _ := json.Marshal(m[col])
		k += "|" + col + "=" + string(b)
	}
	return k
}

// forEachEntity calls fn for every struct in the statement's reflect value.
func forEachEntity(rv reflect.Value, fn func(interface{})) {
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		if rv.CanAddr() {
			fn(rv.Addr().Interface())
		} else {
			cp := reflect.New(rv.Type())
			cp.Elem().Set(rv)
			fn(cp.Interface())
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			forEachEntity(rv.Index(i), fn)
		}
	}
}

func (p *Producer) afterCreate(tx *gorm.DB) {
	if tx.Error != nil || tx.RowsAffected == 0 {
		return
	}
	table, ok := p.table(tx)
	if !ok {
		return
	}
	if tx.Statement.Schema == nil {
		// db.Table("x").Create(map / []map): decode the column maps into models.
		for _, m := range columnMaps(tx.Statement.Dest) {
			b, err := json.Marshal(m)
			if err != nil {
				continue
			}
			if e, err := p.reg.Decode(table, b); err == nil {
				p.emit(table, service.SyncActionCreate, e)
			}
		}
		return
	}
	forEachEntity(tx.Statement.ReflectValue, func(e interface{}) {
		p.emit(table, service.SyncActionCreate, e)
	})
}

// columnMaps returns the column maps of a model-less Create.
func columnMaps(dest interface{}) []map[string]interface{} {
	switch v := dest.(type) {
	case map[string]interface{}:
		return []map[string]interface{}{v}
	case *map[string]interface{}:
		if v != nil {
			return []map[string]interface{}{*v}
		}
	case []map[string]interface{}:
		return v
	case *[]map[string]interface{}:
		if v != nil {
			return *v
		}
	}
	return nil
}

// primaryIDs returns the non-zero primary key values present in the statement's value.
func primaryIDs(tx *gorm.DB) []interface{} {
	var ids []interface{}
	if tx.Statement.Schema == nil {
		return nil
	}
	pf := tx.Statement.Schema.PrioritizedPrimaryField
	if pf == nil {
		return nil
	}
	rv := tx.Statement.ReflectValue
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	collect := func(v reflect.Value) {
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct || v.Type() != tx.Statement.Schema.ModelType {
			return
		}
		if val, zero := pf.ValueOf(tx.Statement.Context, v); !zero {
			ids = append(ids, val)
		}
	}
	switch rv.Kind() {
	case reflect.Struct:
		collect(rv)
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			collect(rv.Index(i))
		}
	}
	return ids
}

// affectedRows loads the rows a pending update/delete will touch, using the
// statement's own connection (transaction-safe) and WHERE clause.
func (p *Producer) affectedRows(tx *gorm.DB, table string) []interface{} {
	sch := p.schemaOf(tx, table)
	if sch == nil {
		return nil
	}
	q := tx.Session(&gorm.Session{NewDB: true}).Model(reflect.New(sch.ModelType).Interface())
	if ids := primaryIDs(tx); len(ids) > 0 && sch.PrioritizedPrimaryField != nil {
		q = q.Where(clause.IN{Column: clause.Column{Name: sch.PrioritizedPrimaryField.DBName}, Values: ids})
	} else if c, ok := tx.Statement.Clauses["WHERE"]; ok {
		if w, ok := c.Expression.(clause.Where); ok && len(w.Exprs) > 0 {
			q = q.Clauses(clause.Where{Exprs: w.Exprs})
		} else {
			return nil
		}
	} else {
		return nil // global update/delete is not tracked
	}
	if tx.Statement.Unscoped {
		q = q.Unscoped()
	}
	slice := reflect.New(reflect.SliceOf(reflect.PointerTo(sch.ModelType)))
	if err := q.Find(slice.Interface()).Error; err != nil {
		p.log.Debug("datasync: load affected rows failed", zap.String("table", table), zap.Error(err))
		return nil
	}
	out := make([]interface{}, 0, slice.Elem().Len())
	for i := 0; i < slice.Elem().Len(); i++ {
		out = append(out, slice.Elem().Index(i).Interface())
	}
	return out
}

func (p *Producer) beforeUpdate(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}
	table, ok := p.table(tx)
	if !ok {
		return
	}
	rows := p.affectedRows(tx, table)
	if len(rows) > 0 {
		tx.InstanceSet(instUpdateRows, rows)
	}
}

func (p *Producer) afterUpdate(tx *gorm.DB) {
	if tx.Error != nil || tx.RowsAffected == 0 {
		return
	}
	table, ok := p.table(tx)
	if !ok {
		return
	}
	v, ok := tx.InstanceGet(instUpdateRows)
	if !ok {
		return
	}
	rows, _ := v.([]interface{})
	sch := p.schemaOf(tx, table)
	if sch == nil || sch.PrioritizedPrimaryField == nil {
		return
	}
	pf := sch.PrioritizedPrimaryField
	if pf == nil {
		return
	}
	ids := make([]interface{}, 0, len(rows))
	for _, r := range rows {
		if val, zero := pf.ValueOf(tx.Statement.Context, reflect.ValueOf(r).Elem()); !zero {
			ids = append(ids, val)
		}
	}
	if len(ids) == 0 {
		return
	}
	slice := reflect.New(reflect.SliceOf(reflect.PointerTo(sch.ModelType)))
	q := tx.Session(&gorm.Session{NewDB: true}).Model(reflect.New(sch.ModelType).Interface()).Unscoped().
		Where(clause.IN{Column: clause.Column{Name: pf.DBName}, Values: ids})
	if err := q.Find(slice.Interface()).Error; err != nil {
		return
	}
	for i := 0; i < slice.Elem().Len(); i++ {
		p.emit(table, service.SyncActionUpdate, slice.Elem().Index(i).Interface())
	}
}

func (p *Producer) beforeDelete(tx *gorm.DB) {
	if tx.Error != nil {
		return
	}
	table, ok := p.table(tx)
	if !ok {
		return
	}
	rows := p.affectedRows(tx, table)
	if len(rows) > 0 {
		tx.InstanceSet(instDeleteRows, rows)
	}
}

func (p *Producer) afterDelete(tx *gorm.DB) {
	if tx.Error != nil || tx.RowsAffected == 0 {
		return
	}
	table, ok := p.table(tx)
	if !ok {
		return
	}
	v, ok := tx.InstanceGet(instDeleteRows)
	if !ok {
		return
	}
	rows, _ := v.([]interface{})
	for _, r := range rows {
		p.emit(table, service.SyncActionDelete, r)
	}
}

// AsyncSink decouples a sink from the GORM callback (Java @Async push): the
// callback only enqueues, a single worker calls fn in order. The worker runs
// outside the writer's transaction, so fn may query the database freely.
type AsyncSink struct {
	mu     sync.RWMutex
	closed bool
	ch     chan *ChangeEvent
	fn     ChangeSink
	done   chan struct{}
}

// NewAsyncSink starts the worker; size is the buffer length (default 4096).
func NewAsyncSink(fn ChangeSink, size int) *AsyncSink {
	if size <= 0 {
		size = 4096
	}
	a := &AsyncSink{ch: make(chan *ChangeEvent, size), fn: fn, done: make(chan struct{})}
	go func() {
		defer close(a.done)
		for ev := range a.ch {
			func() {
				defer func() { _ = recover() }()
				a.fn(ev)
			}()
		}
	}()
	return a
}

// Push enqueues ev (blocks when the buffer is full; dropped after Stop).
func (a *AsyncSink) Push(ev *ChangeEvent) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return
	}
	a.ch <- ev
}

// Stop drains the buffer and stops the worker.
func (a *AsyncSink) Stop() {
	a.mu.Lock()
	if !a.closed {
		a.closed = true
		close(a.ch)
	}
	a.mu.Unlock()
	<-a.done
}
