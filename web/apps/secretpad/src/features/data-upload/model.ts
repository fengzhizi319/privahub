/**
 * Upload flow model: `data/upload` result + edited schema → Java `CreateDataRequest`.
 */
import { DEFAULT_DATASOURCE_ID } from '@secretpad/api-client';
import type { CreateDataRequestJava, UploadDataResultVO } from '@secretpad/api-client';
import { hasWhitespace, parseNullStrs, validateDisplayName } from '@secretpad/utils';
import type { NameIssue } from '@secretpad/utils';
import { isSchemaValid, toDatatableSchema } from '../datatable-schema/model';
import type { SchemaField } from '../datatable-schema/model';

export interface UploadFormValues {
  tableName: string;
  description: string;
  nullStrsText: string;
  fields: SchemaField[];
}

export type UploadFileIssue = 'notCsv' | 'whitespace' | null;

/** Legacy uploader accepted `.csv` only and rejected file names containing whitespace. */
export function checkUploadFile(file: { name: string }): UploadFileIssue {
  if (!/\.csv$/i.test(file.name)) return 'notCsv';
  if (hasWhitespace(file.name)) return 'whitespace';
  return null;
}

export interface UploadFormIssues {
  tableName?: NameIssue;
  description?: 'tooLong';
  nullStrs?: 'invalid';
  schema?: 'invalid';
}

export function validateUploadForm(v: UploadFormValues): UploadFormIssues {
  const issues: UploadFormIssues = {};
  const n = validateDisplayName(v.tableName);
  if (n) issues.tableName = n;
  if (v.description.length > 200) issues.description = 'tooLong';
  if (parseNullStrs(v.nullStrsText) === null) issues.nullStrs = 'invalid';
  if (!isSchemaValid(v.fields)) issues.schema = 'invalid';
  return issues;
}

/** Build Java `CreateDataRequest` for `data/create`. */
export function buildCreateDataRequest(input: {
  nodeId: string;
  fileName: string;
  upload: UploadDataResultVO;
  values: UploadFormValues;
}): CreateDataRequestJava {
  const { nodeId, fileName, upload, values } = input;
  return {
    nodeId,
    // Java: "The data file name, it must be the same as that of the source file".
    name: upload.name || fileName,
    realName: upload.realName,
    tableName: values.tableName.trim(),
    description: values.description.trim(),
    // Upload returns the Kuscia type "localfs"; the platform datatable type is LOCAL
    // (legacy UI registered uploads as LOCAL / default-data-source).
    datasourceType: !upload.datasourceType || /^(localfs|local)$/i.test(upload.datasourceType) ? 'LOCAL' : upload.datasourceType,
    datasourceName: upload.datasource || DEFAULT_DATASOURCE_ID,
    datatableSchema: toDatatableSchema(values.fields),
    nullStrs: parseNullStrs(values.nullStrsText) ?? [],
  };
}
