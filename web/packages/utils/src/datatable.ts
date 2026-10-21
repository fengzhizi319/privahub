/**
 * Pure helpers for datatable registration / upload (schema editing, SQL
 * keyword checks, null-value parsing, partial-failure formatting).
 *
 * Ported from the legacy SecretPad modules `data-table-add` (sql-keyword.ts,
 * data-table-structure, upload-table) and `data-source-list`.
 */

/**
 * MySQL 8.0 keyword list used by the legacy platform to warn about column
 * names that SCQL cannot use
 * (https://dev.mysql.com/doc/refman/8.0/en/keywords.html).
 */
const SQL_KEYWORD_TEXT = `
  ACCESSIBLE ACCOUNT ACTION ACTIVE ADD ADMIN AFTER AGAINST AGGREGATE ALGORITHM ALL ALTER
  ALWAYS ANALYSE ANALYZE AND ANY ARRAY AS ASC ASCII ASENSITIVE AT ATTRIBUTE
  AUTHENTICATION AUTOEXTEND_SIZE AUTO_INCREMENT AVG AVG_ROW_LENGTH BACKUP BEFORE BEGIN
  BETWEEN BIGINT BINARY BINLOG BIT BLOB BLOCK BOOL BOOLEAN BOTH BTREE BUCKETS BULK BY
  BYTE CACHE CALL CASCADE CASCADED CASE CATALOG_NAME CHAIN CHALLENGE_RESPONSE CHANGE
  CHANGED CHANNEL CHAR CHARACTER CHARSET CHECK CHECKSUM CIPHER CLASS_ORIGIN CLIENT CLONE
  CLOSE COALESCE CODE COLLATE COLLATION COLUMN COLUMNS COLUMN_FORMAT COLUMN_NAME COMMENT
  COMMIT COMMITTED COMPACT COMPLETION COMPONENT COMPRESSED COMPRESSION CONCURRENT
  CONDITION CONNECTION CONSISTENT CONSTRAINT CONSTRAINT_CATALOG CONSTRAINT_NAME
  CONSTRAINT_SCHEMA CONTAINS CONTEXT CONTINUE CONVERT CPU CREATE CROSS CUBE CUME_DIST
  CURRENT CURRENT_DATE CURRENT_TIME CURRENT_TIMESTAMP CURRENT_USER CURSOR CURSOR_NAME
  DATA DATABASE DATABASES DATAFILE DATE DATETIME DAY DAY_HOUR DAY_MICROSECOND DAY_MINUTE
  DAY_SECOND DEALLOCATE DEC DECIMAL DECLARE DEFAULT DEFAULT_AUTH DEFINER DEFINITION
  DELAYED DELAY_KEY_WRITE DELETE DENSE_RANK DESC DESCRIBE DESCRIPTION DES_KEY_FILE
  DETERMINISTIC DIAGNOSTICS DIRECTORY DISABLE DISCARD DISK DISTINCT DISTINCTROW DIV DO
  DOUBLE DROP DUAL DUMPFILE DUPLICATE DYNAMIC EACH ELSE ELSEIF EMPTY ENABLE ENCLOSED
  ENCRYPTION END ENDS ENFORCED ENGINE ENGINES ENGINE_ATTRIBUTE ENUM ERROR ERRORS ESCAPE
  ESCAPED EVENT EVENTS EVERY EXCEPT EXCHANGE EXCLUDE EXECUTE EXISTS EXIT EXPANSION EXPIRE
  EXPLAIN EXPORT EXTENDED EXTENT_SIZE FACTOR FAILED_LOGIN_ATTEMPTS FALSE FAST FAULTS
  FETCH FIELDS FILE FILE_BLOCK_SIZE FILTER FINISH FIRST FIRST_VALUE FIXED FLOAT FLOAT4
  FLOAT8 FLUSH FOLLOWING FOLLOWS FOR FORCE FOREIGN FORMAT FOUND FROM FULL FULLTEXT
  FUNCTION GENERAL GENERATE GENERATED GEOMCOLLECTION GEOMETRY GEOMETRYCOLLECTION GET
  GET_FORMAT GET_MASTER_PUBLIC_KEY GET_SOURCE_PUBLIC_KEY GLOBAL GRANT GRANTS GROUP
  GROUPING GROUPS GROUP_REPLICATION GTID_ONLY HANDLER HASH HAVING HELP HIGH_PRIORITY
  HISTOGRAM HISTORY HOST HOSTS HOUR HOUR_MICROSECOND HOUR_MINUTE HOUR_SECOND IDENTIFIED
  IF IGNORE IGNORE_SERVER_IDS IMPORT IN INACTIVE INDEX INDEXES INFILE INITIAL
  INITIAL_SIZE INITIATE INNER INOUT INSENSITIVE INSERT INSERT_METHOD INSTALL INSTANCE INT
  INT1 INT2 INT3 INT4 INT8 INTEGER INTERSECT INTERVAL INTO INVISIBLE INVOKER IO
  IO_AFTER_GTIDS IO_BEFORE_GTIDS IO_THREAD IPC IS ISOLATION ISSUER ITERATE JOIN JSON
  JSON_TABLE JSON_VALUE KEY KEYRING KEYS KEY_BLOCK_SIZE KILL LAG LANGUAGE LAST LAST_VALUE
  LATERAL LEAD LEADING LEAVE LEAVES LEFT LESS LEVEL LIKE LIMIT LINEAR LINES LINESTRING
  LIST LOAD LOCAL LOCALTIME LOCALTIMESTAMP LOCK LOCKED LOCKS LOGFILE LOGS LONG LONGBLOB
  LONGTEXT LOOP LOW_PRIORITY MASTER MASTER_AUTO_POSITION MASTER_BIND
  MASTER_COMPRESSION_ALGORITHMS MASTER_CONNECT_RETRY MASTER_DELAY MASTER_HEARTBEAT_PERIOD
  MASTER_HOST MASTER_LOG_FILE MASTER_LOG_POS MASTER_PASSWORD MASTER_PORT
  MASTER_PUBLIC_KEY_PATH MASTER_RETRY_COUNT MASTER_SERVER_ID MASTER_SSL MASTER_SSL_CA
  MASTER_SSL_CAPATH MASTER_SSL_CERT MASTER_SSL_CIPHER MASTER_SSL_CRL MASTER_SSL_CRLPATH
  MASTER_SSL_KEY MASTER_SSL_VERIFY_SERVER_CERT MASTER_TLS_CIPHERSUITES MASTER_TLS_VERSION
  MASTER_USER MASTER_ZSTD_COMPRESSION_LEVEL MATCH MAXVALUE MAX_CONNECTIONS_PER_HOUR
  MAX_QUERIES_PER_HOUR MAX_ROWS MAX_SIZE MAX_UPDATES_PER_HOUR MAX_USER_CONNECTIONS MEDIUM
  MEDIUMBLOB MEDIUMINT MEDIUMTEXT MEMBER MEMORY MERGE MESSAGE_TEXT MICROSECOND MIDDLEINT
  MIGRATE MINUTE MINUTE_MICROSECOND MINUTE_SECOND MIN_ROWS MOD MODE MODIFIES MODIFY MONTH
  MULTILINESTRING MULTIPOINT MULTIPOLYGON MUTEX MYSQL_ERRNO NAME NAMES NATIONAL NATURAL
  NCHAR NDB NDBCLUSTER NESTED NETWORK_NAMESPACE NEVER NEW NEXT NO NODEGROUP NONE NOT
  NOWAIT NO_WAIT NO_WRITE_TO_BINLOG NTH_VALUE NTILE NULL NULLS NUMBER NUMERIC NVARCHAR OF
  OFF OFFSET OJ OLD ON ONE ONLY OPEN OPTIMIZE OPTIMIZER_COSTS OPTION OPTIONAL OPTIONALLY
  OPTIONS OR ORDER ORDINALITY ORGANIZATION OTHERS OUT OUTER OUTFILE OVER OWNER PACK_KEYS
  PAGE PARSER PARTIAL PARTITION PARTITIONING PARTITIONS PASSWORD PASSWORD_LOCK_TIME PATH
  PERCENT_RANK PERSIST PERSIST_ONLY PHASE PLUGIN PLUGINS PLUGIN_DIR POINT POLYGON PORT
  PRECEDES PRECEDING PRECISION PREPARE PRESERVE PREV PRIMARY PRIVILEGES
  PRIVILEGE_CHECKS_USER PROCEDURE PROCESS PROCESSLIST PROFILE PROFILES PROXY PURGE
  QUARTER QUERY QUICK RANDOM RANGE RANK READ READS READ_ONLY READ_WRITE REAL REBUILD
  RECOVER RECURSIVE REDOFILE REDO_BUFFER_SIZE REDUNDANT REFERENCE REFERENCES REGEXP
  REGISTRATION RELAY RELAYLOG RELAY_LOG_FILE RELAY_LOG_POS RELAY_THREAD RELEASE RELOAD
  REMOTE REMOVE RENAME REORGANIZE REPAIR REPEAT REPEATABLE REPLACE REPLICA REPLICAS
  REPLICATE_DO_DB REPLICATE_DO_TABLE REPLICATE_IGNORE_DB REPLICATE_IGNORE_TABLE
  REPLICATE_REWRITE_DB REPLICATE_WILD_DO_TABLE REPLICATE_WILD_IGNORE_TABLE REPLICATION
  REQUIRE REQUIRE_ROW_FORMAT RESET RESIGNAL RESOURCE RESPECT RESTART RESTORE RESTRICT
  RESUME RETAIN RETURN RETURNED_SQLSTATE RETURNING RETURNS REUSE REVERSE REVOKE RIGHT
  RLIKE ROLE ROLLBACK ROLLUP ROTATE ROUTINE ROW ROWS ROW_COUNT ROW_FORMAT ROW_NUMBER
  RTREE SAVEPOINT SCHEDULE SCHEMA SCHEMAS SCHEMA_NAME SECOND SECONDARY SECONDARY_ENGINE
  SECONDARY_ENGINE_ATTRIBUTE SECONDARY_LOAD SECONDARY_UNLOAD SECOND_MICROSECOND SECURITY
  SELECT SENSITIVE SEPARATOR SERIAL SERIALIZABLE SERVER SESSION SET SHARE SHOW SHUTDOWN
  SIGNAL SIGNED SIMPLE SKIP SLAVE SLOW SMALLINT SNAPSHOT SOCKET SOME SONAME SOUNDS SOURCE
  SOURCE_AUTO_POSITION SOURCE_BIND SOURCE_COMPRESSION_ALGORITHMS SOURCE_CONNECT_RETRY
  SOURCE_DELAY SOURCE_HEARTBEAT_PERIOD SOURCE_HOST SOURCE_LOG_FILE SOURCE_LOG_POS
  SOURCE_PASSWORD SOURCE_PORT SOURCE_PUBLIC_KEY_PATH SOURCE_RETRY_COUNT SOURCE_SSL
  SOURCE_SSL_CA SOURCE_SSL_CAPATH SOURCE_SSL_CERT SOURCE_SSL_CIPHER SOURCE_SSL_CRL
  SOURCE_SSL_CRLPATH SOURCE_SSL_KEY SOURCE_SSL_VERIFY_SERVER_CERT SOURCE_TLS_CIPHERSUITES
  SOURCE_TLS_VERSION SOURCE_USER SOURCE_ZSTD_COMPRESSION_LEVEL SPATIAL SPECIFIC SQL
  SQLEXCEPTION SQLSTATE SQLWARNING SQL_AFTER_GTIDS SQL_AFTER_MTS_GAPS SQL_BEFORE_GTIDS
  SQL_BIG_RESULT SQL_BUFFER_RESULT SQL_CACHE SQL_CALC_FOUND_ROWS SQL_NO_CACHE
  SQL_SMALL_RESULT SQL_THREAD SQL_TSI_DAY SQL_TSI_HOUR SQL_TSI_MINUTE SQL_TSI_MONTH
  SQL_TSI_QUARTER SQL_TSI_SECOND SQL_TSI_WEEK SQL_TSI_YEAR SRID SSL STACKED START
  STARTING STARTS STATS_AUTO_RECALC STATS_PERSISTENT STATS_SAMPLE_PAGES STATUS STOP
  STORAGE STORED STRAIGHT_JOIN STREAM STRING SUBCLASS_ORIGIN SUBJECT SUBPARTITION
  SUBPARTITIONS SUPER SUSPEND SWAPS SWITCHES SYSTEM TABLE TABLES TABLESPACE
  TABLE_CHECKSUM TABLE_NAME TEMPORARY TEMPTABLE TERMINATED TEXT THAN THEN THREAD_PRIORITY
  TIES TIME TIMESTAMP TIMESTAMPADD TIMESTAMPDIFF TINYBLOB TINYINT TINYTEXT TLS TO
  TRAILING TRANSACTION TRIGGER TRIGGERS TRUE TRUNCATE TYPE TYPES UNBOUNDED UNCOMMITTED
  UNDEFINED UNDO UNDOFILE UNDO_BUFFER_SIZE UNICODE UNINSTALL UNION UNIQUE UNKNOWN UNLOCK
  UNREGISTER UNSIGNED UNTIL UPDATE UPGRADE URL USAGE USE USER USER_RESOURCES USE_FRM
  USING UTC_DATE UTC_TIME UTC_TIMESTAMP VALIDATION VALUE VALUES VARBINARY VARCHAR
  VARCHARACTER VARIABLES VARYING VCPU VIEW VIRTUAL VISIBLE WAIT WARNINGS WEEK
  WEIGHT_STRING WHEN WHERE WHILE WINDOW WITH WITHOUT WORK WRAPPER WRITE X509 XA XID XML
  XOR YEAR YEAR_MONTH ZEROFILL ZONE
`;

export const SQL_KEYWORDS: readonly string[] = SQL_KEYWORD_TEXT.split(/\s+/).filter(Boolean);

const SQL_KEYWORD_SET = new Set(SQL_KEYWORDS.map((k) => k.toLowerCase()));

/** Whether `word` is a (MySQL) SQL keyword, case-insensitive. */
export function isSqlKeyword(word?: string | null): boolean {
  if (!word) return false;
  return SQL_KEYWORD_SET.has(word.trim().toLowerCase());
}

/** SCQL cannot use a column that is a SQL keyword or contains a hyphen. */
export function isScqlUnfriendly(word?: string | null): boolean {
  if (!word) return false;
  return isSqlKeyword(word) || word.includes('-');
}

/* ------------------------------------------------------------------ */
/* Schema (feature) editing                                            */
/* ------------------------------------------------------------------ */

/** Column types offered by the legacy schema editor (value → display label). */
export const SCHEMA_TYPE_OPTIONS = [
  { value: 'int', label: 'integer' },
  { value: 'float', label: 'float' },
  { value: 'str', label: 'string' },
] as const;

export type SchemaColType = (typeof SCHEMA_TYPE_OPTIONS)[number]['value'];

/** One editable schema row (Java `DatatableSchema` naming). */
export interface SchemaField {
  featureName: string;
  featureType: string;
  featureDescription: string;
}

export type SchemaFieldIssueKind =
  | 'nameRequired'
  | 'nameTooLong'
  | 'namePattern'
  | 'duplicate'
  | 'typeRequired'
  | 'descTooLong';

export interface SchemaFieldIssue {
  index: number;
  kind: SchemaFieldIssueKind;
}

export const FEATURE_NAME_MAX = 64;
export const FEATURE_DESC_MAX = 200;
/** Legacy rule: English letter first, then letters / digits / `_` / `-`. */
export const FEATURE_NAME_RE = /^[a-zA-Z][a-zA-Z0-9_-]*$/;

/**
 * Validate schema rows exactly like the legacy antd form rules: required
 * name (<=64, pattern), no duplicates, required type, description <=200.
 */
export function validateSchemaFields(fields: SchemaField[]): SchemaFieldIssue[] {
  const issues: SchemaFieldIssue[] = [];
  const counts = new Map<string, number>();
  fields.forEach((f) => {
    const n = (f.featureName || '').trim();
    if (n) counts.set(n, (counts.get(n) || 0) + 1);
  });
  fields.forEach((f, index) => {
    const name = (f.featureName || '').trim();
    if (!name) issues.push({ index, kind: 'nameRequired' });
    else {
      if (name.length > FEATURE_NAME_MAX) issues.push({ index, kind: 'nameTooLong' });
      if (!FEATURE_NAME_RE.test(name)) issues.push({ index, kind: 'namePattern' });
      if ((counts.get(name) || 0) > 1) issues.push({ index, kind: 'duplicate' });
    }
    if (!f.featureType) issues.push({ index, kind: 'typeRequired' });
    if ((f.featureDescription || '').length > FEATURE_DESC_MAX) issues.push({ index, kind: 'descTooLong' });
  });
  return issues;
}

/** Indexes of rows whose name is valid but unusable by SCQL (warning only). */
export function scqlWarningIndexes(fields: SchemaField[]): number[] {
  return fields.map((f, i) => (isScqlUnfriendly(f.featureName?.trim()) ? i : -1)).filter((i) => i >= 0);
}

/** Map an inferred CSV type onto the editor's type options (bool → str). */
export function toSchemaColType(t: string): SchemaColType {
  if (t === 'int' || t === 'float' || t === 'str') return t;
  if (t === 'integer') return 'int';
  if (t === 'string') return 'str';
  return 'str';
}

/** Header of the legacy schema template CSV. */
export const SCHEMA_TEMPLATE_HEADER = ['特征名称', '特征类型', '特征描述'] as const;

/** Legacy sample template rows (label-typed, as in the old `示例文件.csv`). */
export const SCHEMA_TEMPLATE_ROWS: readonly [string, string, string][] = [
  ['id1', 'string', ''],
  ['x1', 'integer', '描述'],
  ['x2', 'integer', ''],
  ['x3', 'integer', ''],
  ['x4', 'float', ''],
  ['x5', 'float', ''],
  ['x6', 'float', ''],
  ['x7', 'float', ''],
  ['x8', 'float', ''],
  ['x9', 'float', ''],
  ['x10', 'float', ''],
];

function csvCell(v: string): string {
  return /[",\r\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/** Build the schema template CSV text (UTF-8 BOM so Excel opens it correctly). */
export function buildSchemaTemplateCsv(): string {
  const rows = [SCHEMA_TEMPLATE_HEADER as readonly string[], ...SCHEMA_TEMPLATE_ROWS];
  return '\uFEFF' + rows.map((r) => r.map(csvCell).join(',')).join('\n') + '\n';
}

export interface SchemaCsvResult {
  fields: SchemaField[];
  /** Number of rows dropped because the feature name was repeated. */
  duplicates: number;
}

/**
 * Parse an uploaded schema CSV (`特征名称,特征类型,特征描述`). Returns null
 * when the header does not match the template. Type labels (integer/float/
 * string) map to values; unknown types become ''. Duplicate names are dropped.
 */
export function parseSchemaCsv(rows: { header: string[]; rows: string[][] }): SchemaCsvResult | null {
  const header = rows.header.map((h) => h.trim());
  if (header.length !== 3) return null;
  if (header.some((h) => h && !(SCHEMA_TEMPLATE_HEADER as readonly string[]).includes(h))) return null;
  const idx = (k: string) => header.indexOf(k);
  const iName = idx('特征名称');
  const iType = idx('特征类型');
  const iDesc = idx('特征描述');
  const seen = new Set<string>();
  let duplicates = 0;
  const fields: SchemaField[] = [];
  for (const r of rows.rows) {
    const featureName = (iName >= 0 ? r[iName] : '')?.trim() || '';
    if (!featureName) continue;
    if (seen.has(featureName)) {
      duplicates++;
      continue;
    }
    seen.add(featureName);
    const rawType = ((iType >= 0 ? r[iType] : '') || '').trim();
    const opt = SCHEMA_TYPE_OPTIONS.find((o) => o.label === rawType || o.value === rawType);
    fields.push({
      featureName,
      featureType: opt ? opt.value : '',
      featureDescription: ((iDesc >= 0 ? r[iDesc] : '') || '').trim(),
    });
  }
  return { fields, duplicates };
}

/* ------------------------------------------------------------------ */
/* Names                                                               */
/* ------------------------------------------------------------------ */

/** Datatable / datasource display name rule: CJK, letters, digits, `_`, `-`; max 32. */
export const DISPLAY_NAME_RE = /^[\u4e00-\u9fa5A-Za-z0-9_-]+$/;
export const DISPLAY_NAME_MAX = 32;

export type NameIssue = 'required' | 'tooLong' | 'pattern';

export function validateDisplayName(name: string, max = DISPLAY_NAME_MAX): NameIssue | null {
  const v = (name || '').trim();
  if (!v) return 'required';
  if (v.length > max) return 'tooLong';
  if (!DISPLAY_NAME_RE.test(v)) return 'pattern';
  return null;
}

/** File names with whitespace were rejected by the legacy uploader. */
export function hasWhitespace(fileName: string): boolean {
  return /\s/.test(fileName);
}

/* ------------------------------------------------------------------ */
/* Null strings                                                        */
/* ------------------------------------------------------------------ */

/** Default value of the legacy "空缺值" textarea. */
export const DEFAULT_NULL_STRS_TEXT = '""';

/**
 * Parse the legacy null-string text (comma separated JSON strings, e.g.
 * `"", "NA", "-999"`). Returns null when the text is not valid. Empty text → [].
 */
export function parseNullStrs(text: string): string[] | null {
  const v = (text || '').trim();
  if (!v) return [];
  try {
    const arr = JSON.parse(`[${v}]`) as unknown;
    if (!Array.isArray(arr)) return null;
    return arr.map((x) => String(x));
  } catch {
    return null;
  }
}

/** Render null strings back to the legacy text form (`"","NA"`). */
export function formatNullStrs(values?: string[] | null): string {
  return (values || []).map((v) => JSON.stringify(v)).join(',');
}

/* ------------------------------------------------------------------ */
/* Partial failures                                                    */
/* ------------------------------------------------------------------ */

export interface FailedNode {
  nodeId: string;
  message: string;
}

/**
 * Normalise Java `failedCreatedNodes` (`Record<nodeId, errorMessage>`) into a
 * list. Non-string messages are stringified; empty/nullish input → [].
 */
export function formatFailedNodes(failed?: Record<string, unknown> | null): FailedNode[] {
  if (!failed || typeof failed !== 'object') return [];
  return Object.entries(failed).map(([nodeId, msg]) => ({
    nodeId,
    message: typeof msg === 'string' ? msg : msg == null ? '' : JSON.stringify(msg),
  }));
}
