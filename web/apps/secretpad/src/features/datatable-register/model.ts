/**
 * Datatable registration from an existing datasource (legacy
 * `data-table-add/add-data` + `add-data-service`). Pure payload builders.
 *
 * Per type:
 * - OSS:   relativeUri = file path relative to the datasource prefix
 * - MYSQL: relativeUri = source table name
 * - ODPS:  relativeUri = ODPS table name, `partition = {type:'odps', fields}`
 * - HTTP:  relativeUri = HTTP address, datasource id/name `http-data-source`, no nullStrs
 */
import { HTTP_DATASOURCE_ID } from '@secretpad/api-client';
import type { CreateDatatableRequestJava } from '@secretpad/api-client';
import { isSqlKeyword, parseNullStrs, validateDisplayName } from '@secretpad/utils';
import type { NameIssue } from '@secretpad/utils';
import { isSchemaValid, toTableColumns } from '../datatable-schema/model';
import type { SchemaField } from '../datatable-schema/model';

export const MAX_REGISTER_NODES = 5;
export const TABLE_DESC_MAX = 100;

export interface RegisterDatasource {
  datasourceId: string;
  name: string;
  type: string;
}

export interface RegisterFormValues {
  /** OSS path / MYSQL table / HTTP address / ODPS table. */
  address: string;
  tableName: string;
  description: string;
  nullStrsText: string;
  fields: SchemaField[];
  /** ODPS partition column names (level 1, level 2). */
  partitions: string[];
}

export const emptyRegisterForm = (): RegisterFormValues => ({
  address: '',
  tableName: '',
  description: '',
  nullStrsText: '""',
  fields: [{ featureName: '', featureType: '', featureDescription: '' }],
  partitions: ['', ''],
});

/** Types that support the "null strings" option (legacy: OSS / ODPS / MYSQL). */
export function supportsNullStrs(type?: string): boolean {
  return type === 'OSS' || type === 'ODPS' || type === 'MYSQL';
}

export interface RegisterFormIssues {
  address?: 'required';
  tableName?: NameIssue;
  description?: 'tooLong';
  nullStrs?: 'invalid';
  schema?: 'invalid';
  nodeIds?: 'required' | 'tooMany';
  partitions?: 'unknownColumn';
}

export function validateRegisterForm(
  type: string,
  v: RegisterFormValues,
  nodeIds: string[],
  opts: { multiNode: boolean },
): RegisterFormIssues {
  const issues: RegisterFormIssues = {};
  if (!v.address.trim()) issues.address = 'required';
  const n = validateDisplayName(v.tableName);
  if (n) issues.tableName = n;
  if (v.description.length > TABLE_DESC_MAX) issues.description = 'tooLong';
  if (supportsNullStrs(type) && parseNullStrs(v.nullStrsText) === null) issues.nullStrs = 'invalid';
  if (!isSchemaValid(v.fields)) issues.schema = 'invalid';
  if (opts.multiNode) {
    if (nodeIds.length === 0) issues.nodeIds = 'required';
    else if (nodeIds.length > MAX_REGISTER_NODES) issues.nodeIds = 'tooMany';
  }
  if (type === 'ODPS') {
    const names = new Set(v.fields.map((f) => f.featureName.trim()));
    if (v.partitions.some((p) => p.trim() && !names.has(p.trim()))) issues.partitions = 'unknownColumn';
  }
  return issues;
}

/** SCQL/SQL keyword warnings for the datatable name and (MYSQL/ODPS) source table name. */
export function tableKeywordWarnings(type: string, v: RegisterFormValues): { tableName: boolean; address: boolean } {
  return {
    tableName: isSqlKeyword(v.tableName.trim()),
    address: (type === 'MYSQL' || type === 'ODPS') && isSqlKeyword(v.address.trim()),
  };
}

/** Build Java `CreateDatatableRequest` for `datatable/create`. */
export function buildCreateDatatableRequest(input: {
  ownerId: string;
  nodeIds: string[];
  datasource: RegisterDatasource;
  values: RegisterFormValues;
}): CreateDatatableRequestJava {
  const { ownerId, nodeIds, datasource, values } = input;
  const type = datasource.type;
  const base: CreateDatatableRequestJava = {
    ownerId,
    nodeIds,
    datatableName: values.tableName.trim(),
    datasourceId: datasource.datasourceId,
    datasourceName: datasource.name,
    datasourceType: type,
    type,
    desc: values.description.trim(),
    relativeUri: values.address.trim(),
    columns: toTableColumns(values.fields),
  };
  if (type === 'HTTP') {
    return { ...base, datasourceId: HTTP_DATASOURCE_ID, datasourceName: HTTP_DATASOURCE_ID };
  }
  const withNull = supportsNullStrs(type) ? { ...base, nullStrs: parseNullStrs(values.nullStrsText) ?? [] } : base;
  if (type === 'ODPS') {
    const fields = values.partitions
      .map((p) => p.trim())
      .filter(Boolean)
      .map((name) => values.fields.find((f) => f.featureName.trim() === name))
      .filter((f): f is SchemaField => !!f)
      .map((f) => ({ name: f.featureName.trim(), type: f.featureType, comment: f.featureDescription }));
    return { ...withNull, partition: { type: 'odps', fields } };
  }
  return withNull;
}
