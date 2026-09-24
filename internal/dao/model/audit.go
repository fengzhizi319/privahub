package model

// AuditLogDO is one tamper-evident security audit record (internal/audit).
//
// Every row is linked to its predecessor: Hash = SM3(len-prefixed canonical
// encoding of all content fields + PrevHash). Seq is the gap-free chain
// position (deleting a row leaves a gap that verification reports). The
// table is append-only: no API updates or deletes rows; only the retention
// archiver removes a contiguous prefix after exporting it to a segment file
// and recording the boundary in audit_segment.
type AuditLogDO struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Seq          int64  `gorm:"column:seq;uniqueIndex:uk_audit_log_seq;not null" json:"seq"`
	EventID      string `gorm:"column:event_id;type:varchar(64);uniqueIndex:uk_audit_log_event_id;not null" json:"eventId"`
	TsMicro      int64  `gorm:"column:ts_micro;index:idx_audit_log_ts;not null" json:"tsMicro"`
	Actor        string `gorm:"column:actor;type:varchar(128);index:idx_audit_log_actor" json:"actor"`
	ActorType    string `gorm:"column:actor_type;type:varchar(16)" json:"actorType"`
	OwnerID      string `gorm:"column:owner_id;type:varchar(64)" json:"ownerId"`
	IP           string `gorm:"column:ip;type:varchar(64)" json:"ip"`
	Action       string `gorm:"column:action;type:varchar(255);index:idx_audit_log_action" json:"action"`
	ResourceType string `gorm:"column:resource_type;type:varchar(64)" json:"resourceType"`
	ResourceID   string `gorm:"column:resource_id;type:varchar(255);index:idx_audit_log_resource" json:"resourceId"`
	Result       string `gorm:"column:result;type:varchar(16);index:idx_audit_log_result" json:"result"`
	Reason       string `gorm:"column:reason;type:varchar(512)" json:"reason"`
	TraceID      string `gorm:"column:trace_id;type:varchar(128)" json:"traceId"`
	Extra        string `gorm:"column:extra;type:text" json:"extra"`
	PrevHash     string `gorm:"column:prev_hash;type:varchar(64);not null" json:"prevHash"`
	Hash         string `gorm:"column:hash;type:varchar(64);not null" json:"hash"`
}

func (AuditLogDO) TableName() string { return "audit_log" }

// AuditChainTailDO is the single-row (id=1) chain tail. Writers lock it
// (SELECT ... FOR UPDATE on MySQL) and advance it with a compare-and-set in
// the same transaction as the insert, so concurrent writers (several
// processes on one database) can never fork the chain.
type AuditChainTailDO struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Seq         int64  `gorm:"column:seq;not null" json:"seq"`
	Hash        string `gorm:"column:hash;type:varchar(64);not null" json:"hash"`
	UpdateMicro int64  `gorm:"column:update_micro" json:"updateMicro"`
}

func (AuditChainTailDO) TableName() string { return "audit_chain_tail" }

// AuditSegmentDO records an archived (exported then deleted) contiguous chain
// segment [FirstSeq, LastSeq]. LastHash is the boundary hash the first
// remaining audit_log row must link to. Segments form their own hash chain
// (SegHash over the fields + PrevSegHash).
type AuditSegmentDO struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	SegSeq        int64  `gorm:"column:seg_seq;uniqueIndex:uk_audit_segment_seq;not null" json:"segSeq"`
	FirstSeq      int64  `gorm:"column:first_seq;uniqueIndex:uk_audit_segment_first;not null" json:"firstSeq"`
	LastSeq       int64  `gorm:"column:last_seq;not null" json:"lastSeq"`
	RecordCount   int64  `gorm:"column:record_count;not null" json:"recordCount"`
	FirstPrevHash string `gorm:"column:first_prev_hash;type:varchar(64);not null" json:"firstPrevHash"`
	LastHash      string `gorm:"column:last_hash;type:varchar(64);not null" json:"lastHash"`
	FileName      string `gorm:"column:file_name;type:varchar(255)" json:"fileName"`
	FileSM3       string `gorm:"column:file_sm3;type:varchar(64)" json:"fileSm3"`
	ArchiveMicro  int64  `gorm:"column:archive_micro" json:"archiveMicro"`
	PrevSegHash   string `gorm:"column:prev_seg_hash;type:varchar(64);not null" json:"prevSegHash"`
	SegHash       string `gorm:"column:seg_hash;type:varchar(64);not null" json:"segHash"`
}

func (AuditSegmentDO) TableName() string { return "audit_segment" }
