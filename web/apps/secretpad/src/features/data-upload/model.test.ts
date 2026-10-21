import { describe, expect, it } from 'vitest';
import { buildCreateDataRequest, checkUploadFile, validateUploadForm } from './model';

const values = {
  tableName: ' my_table ',
  description: ' desc ',
  nullStrsText: '"", "NA"',
  fields: [
    { featureName: 'id', featureType: 'str', featureDescription: '' },
    { featureName: 'age', featureType: 'int', featureDescription: 'years' },
  ],
};

describe('data/create payload builder', () => {
  it('builds the Java CreateDataRequest from the upload result', () => {
    const req = buildCreateDataRequest({
      nodeId: 'alice',
      fileName: 'a.csv',
      upload: { name: 'a.csv', realName: 'a_1234.csv', datasource: 'default-data-source', datasourceType: 'localfs' },
      values,
    });
    expect(req).toEqual({
      nodeId: 'alice',
      name: 'a.csv',
      realName: 'a_1234.csv',
      tableName: 'my_table',
      description: 'desc',
      datasourceType: 'LOCAL',
      datasourceName: 'default-data-source',
      datatableSchema: [
        { featureName: 'id', featureType: 'str', featureDescription: '' },
        { featureName: 'age', featureType: 'int', featureDescription: 'years' },
      ],
      nullStrs: ['', 'NA'],
    });
  });

  it('falls back to the file name and local datasource defaults', () => {
    const req = buildCreateDataRequest({ nodeId: 'bob', fileName: 'b.csv', upload: {}, values });
    expect(req.name).toBe('b.csv');
    expect(req.datasourceType).toBe('LOCAL');
    expect(req.datasourceName).toBe('default-data-source');
  });
});

describe('upload validation', () => {
  it('checks files like the legacy uploader', () => {
    expect(checkUploadFile({ name: 'a.csv' })).toBeNull();
    expect(checkUploadFile({ name: 'a b.csv' })).toBe('whitespace');
    expect(checkUploadFile({ name: 'a.txt' })).toBe('notCsv');
  });

  it('validates name, null strings and schema', () => {
    expect(validateUploadForm(values)).toEqual({});
    expect(
      validateUploadForm({ ...values, tableName: '', nullStrsText: 'NA', fields: [{ featureName: '1x', featureType: 'int', featureDescription: '' }] }),
    ).toEqual({ tableName: 'required', nullStrs: 'invalid', schema: 'invalid' });
  });
});
