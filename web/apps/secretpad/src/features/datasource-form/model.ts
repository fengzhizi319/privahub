/**
 * Datasource registration form model (pure; ported from legacy
 * `data-source-list/components/create-data-source`).
 *
 * `dataSourceInfo` shapes per type (Java `DataSourceInfo`):
 * - OSS:   { endpoint, ak, sk, bucket, prefix, virtualhost }
 * - MYSQL: { endpoint: "host:port", user, password, database }
 * - ODPS:  { endpoint, project, accessId, accessKey }  (name gets "ODPS-" prefix)
 * - LOCAL: { path }
 * - HTTP:  {}
 */
import { DISPLAY_NAME_RE, DISPLAY_NAME_MAX } from '@secretpad/utils';
import type { CreateDatasourceRequestJava } from '@secretpad/api-client';

export const DS_FORM_TYPES = ['OSS', 'ODPS', 'MYSQL', 'HTTP', 'LOCAL'] as const;
export type DsFormType = (typeof DS_FORM_TYPES)[number];
/**
 * Types datasource/create accepts (Java/Go: only OSS / ODPS / MYSQL handlers
 * implement create; HTTP is registered via feature_datasource/create and LOCAL
 * is the built-in default-data-source).
 */
export const DS_CREATABLE_TYPES: readonly DsFormType[] = ['OSS', 'ODPS', 'MYSQL'];

/** AUTONOMY may register one datasource to at most this many nodes. */
export const MAX_DATASOURCE_NODES = 5;
export const ODPS_NAME_PREFIX = 'ODPS-';

export interface DatasourceFormValues {
  type: DsFormType;
  name: string;
  // OSS
  ossEndpoint: string;
  ak: string;
  sk: string;
  bucket: string;
  prefix: string;
  virtualhost: boolean;
  // MYSQL
  mysqlHost: string;
  mysqlPort: string;
  mysqlUser: string;
  mysqlPassword: string;
  mysqlDatabase: string;
  // ODPS
  odpsEndpoint: string;
  odpsProject: string;
  accessId: string;
  accessKey: string;
  // LOCAL
  localPath: string;
}

export const emptyDatasourceForm = (type: DsFormType = 'OSS'): DatasourceFormValues => ({
  type,
  name: '',
  ossEndpoint: '',
  ak: '',
  sk: '',
  bucket: '',
  prefix: '',
  virtualhost: false,
  mysqlHost: '',
  mysqlPort: '3306',
  mysqlUser: '',
  mysqlPassword: '',
  mysqlDatabase: '',
  odpsEndpoint: '',
  odpsProject: '',
  accessId: '',
  accessKey: '',
  localPath: '',
});

/** i18n key suffix under `dsForm.err.*`. */
export type DsFieldError =
  | 'required'
  | 'tooLong32'
  | 'tooLong64'
  | 'namePattern'
  | 'bucketLength'
  | 'bucketPattern'
  | 'port'
  | 'databaseLength'
  | 'databasePattern'
  | 'nodesRequired'
  | 'nodesTooMany'
  | 'nodesDuplicate';

export type DsFormErrors = Partial<Record<keyof DatasourceFormValues | 'nodeIds', DsFieldError>>;

const BUCKET_RE = /^[a-z0-9]([a-z0-9-]*)$/;
const DATABASE_RE = /^[a-zA-Z_]([A-Za-z0-9_-]*)$/;

function nameRule(v: string): DsFieldError | undefined {
  const s = v.trim();
  if (!s) return 'required';
  if (s.length > DISPLAY_NAME_MAX) return 'tooLong32';
  if (!DISPLAY_NAME_RE.test(s)) return 'namePattern';
  return undefined;
}

function required(v: string, max?: 64): DsFieldError | undefined {
  const s = v.trim();
  if (!s) return 'required';
  if (max && s.length > max) return 'tooLong64';
  return undefined;
}

/** Field-level validation identical to the legacy antd rules. */
export function validateDatasourceForm(
  v: DatasourceFormValues,
  nodeIds: string[],
  maxNodes = MAX_DATASOURCE_NODES,
): DsFormErrors {
  const e: DsFormErrors = {};
  const set = (k: keyof DsFormErrors, err: DsFieldError | undefined) => {
    if (err) e[k] = err;
  };
  set('name', nameRule(v.name));
  switch (v.type) {
    case 'OSS': {
      set('ossEndpoint', required(v.ossEndpoint, 64));
      set('ak', required(v.ak));
      set('sk', required(v.sk));
      const b = v.bucket.trim();
      if (!b) set('bucket', 'required');
      else if (b.length < 3 || b.length > 63) set('bucket', 'bucketLength');
      else if (!BUCKET_RE.test(b)) set('bucket', 'bucketPattern');
      break;
    }
    case 'MYSQL': {
      set('mysqlHost', required(v.mysqlHost, 64));
      const p = v.mysqlPort.trim();
      if (!p) set('mysqlPort', 'required');
      else if (!/^\d+$/.test(p) || Number(p) < 1 || Number(p) > 65535) set('mysqlPort', 'port');
      else if (`${v.mysqlHost.trim()}:${p}`.length > 64) set('mysqlHost', 'tooLong64');
      set('mysqlUser', required(v.mysqlUser));
      set('mysqlPassword', required(v.mysqlPassword));
      const d = v.mysqlDatabase.trim();
      if (!d) set('mysqlDatabase', 'required');
      else if (d.length < 3 || d.length > 63) set('mysqlDatabase', 'databaseLength');
      else if (!DATABASE_RE.test(d)) set('mysqlDatabase', 'databasePattern');
      break;
    }
    case 'ODPS':
      set('odpsProject', nameRule(v.odpsProject));
      set('odpsEndpoint', required(v.odpsEndpoint, 64));
      set('accessId', required(v.accessId));
      set('accessKey', required(v.accessKey));
      break;
    case 'LOCAL':
      set('localPath', required(v.localPath));
      break;
    case 'HTTP':
      break;
  }
  const ids = nodeIds.filter(Boolean);
  if (ids.length === 0 || ids.length !== nodeIds.length) e.nodeIds = 'nodesRequired';
  else if (ids.length > maxNodes) e.nodeIds = 'nodesTooMany';
  else if (new Set(ids).size !== ids.length) e.nodeIds = 'nodesDuplicate';
  return e;
}

/** Build the Java `dataSourceInfo` for the chosen type. */
export function buildDataSourceInfo(v: DatasourceFormValues): Record<string, unknown> {
  switch (v.type) {
    case 'OSS':
      return {
        endpoint: v.ossEndpoint.trim(),
        ak: v.ak.trim(),
        sk: v.sk,
        bucket: v.bucket.trim(),
        prefix: v.prefix.trim(),
        virtualhost: !!v.virtualhost,
      };
    case 'MYSQL':
      return {
        endpoint: `${v.mysqlHost.trim()}:${v.mysqlPort.trim()}`,
        user: v.mysqlUser.trim(),
        password: v.mysqlPassword,
        database: v.mysqlDatabase.trim(),
      };
    case 'ODPS':
      return {
        endpoint: v.odpsEndpoint.trim(),
        project: v.odpsProject.trim(),
        accessId: v.accessId.trim(),
        accessKey: v.accessKey,
      };
    case 'LOCAL':
      return { path: v.localPath.trim() };
    case 'HTTP':
    default:
      return {};
  }
}

/** Display name sent to the backend (ODPS names are auto-prefixed with "ODPS-"). */
export function datasourceSubmitName(v: DatasourceFormValues): string {
  const n = v.name.trim();
  if (v.type === 'ODPS' && !n.startsWith(ODPS_NAME_PREFIX)) return ODPS_NAME_PREFIX + n;
  return n;
}

/** Java `CreateDatasourceRequest`. */
export function buildCreateDatasourceRequest(
  v: DatasourceFormValues,
  ownerId: string,
  nodeIds: string[],
): CreateDatasourceRequestJava {
  return {
    ownerId,
    nodeIds: nodeIds.filter(Boolean),
    type: v.type,
    name: datasourceSubmitName(v),
    dataSourceInfo: buildDataSourceInfo(v),
  };
}

/** Keys of `info` that must be masked when shown (legacy showed `******`). */
export const SECRET_INFO_KEYS = ['sk', 'accessKey', 'password'];

/** Whether a datasource may be deleted (legacy: HTTP never; bound tables block deletion). */
export function datasourceDeleteBlock(ds: { type?: string; relatedDatas?: string[] }): 'http' | 'bound' | null {
  if (ds.relatedDatas && ds.relatedDatas.length > 0) return 'bound';
  if (ds.type === 'HTTP') return 'http';
  return null;
}
