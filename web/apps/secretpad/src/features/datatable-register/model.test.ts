import { describe, expect, it } from 'vitest';
import { buildCreateDatatableRequest, emptyRegisterForm, tableKeywordWarnings, validateRegisterForm } from './model';
import type { RegisterFormValues } from './model';

const values = (patch: Partial<RegisterFormValues> = {}): RegisterFormValues => ({
  ...emptyRegisterForm(),
  address: 'dir/a.csv',
  tableName: 'tbl_a',
  description: 'd',
  nullStrsText: '"", "NA"',
  fields: [
    { featureName: 'id', featureType: 'str', featureDescription: '' },
    { featureName: 'dt', featureType: 'str', featureDescription: 'day' },
  ],
  ...patch,
});

describe('buildCreateDatatableRequest', () => {
  it('builds OSS requests with nullStrs', () => {
    const req = buildCreateDatatableRequest({
      ownerId: 'alice',
      nodeIds: ['alice'],
      datasource: { datasourceId: 'ds1', name: 'oss1', type: 'OSS' },
      values: values(),
    });
    expect(req).toEqual({
      ownerId: 'alice',
      nodeIds: ['alice'],
      datatableName: 'tbl_a',
      datasourceId: 'ds1',
      datasourceName: 'oss1',
      datasourceType: 'OSS',
      type: 'OSS',
      desc: 'd',
      relativeUri: 'dir/a.csv',
      columns: [
        { colName: 'id', colType: 'str', colComment: '' },
        { colName: 'dt', colType: 'str', colComment: 'day' },
      ],
      nullStrs: ['', 'NA'],
    });
  });

  it('builds ODPS partitions from schema columns', () => {
    const req = buildCreateDatatableRequest({
      ownerId: 'o',
      nodeIds: ['n1', 'n2'],
      datasource: { datasourceId: 'ds2', name: 'ODPS-x', type: 'ODPS' },
      values: values({ address: 'tbl', partitions: ['dt', ''] }),
    });
    expect(req.partition).toEqual({ type: 'odps', fields: [{ name: 'dt', type: 'str', comment: 'day' }] });
    expect(req.relativeUri).toBe('tbl');
    expect(req.nodeIds).toEqual(['n1', 'n2']);
  });

  it('uses the built-in HTTP datasource and no nullStrs for HTTP', () => {
    const req = buildCreateDatatableRequest({
      ownerId: 'o',
      nodeIds: ['o'],
      datasource: { datasourceId: 'x', name: 'x', type: 'HTTP' },
      values: values({ address: 'http://h/q' }),
    });
    expect(req.datasourceId).toBe('http-data-source');
    expect(req.datasourceName).toBe('http-data-source');
    expect(req.nullStrs).toBeUndefined();
  });
});

describe('validateRegisterForm', () => {
  it('accepts a valid form', () => {
    expect(validateRegisterForm('OSS', values(), ['a'], { multiNode: false })).toEqual({});
  });

  it('reports missing fields, node limits and bad partitions', () => {
    const issues = validateRegisterForm(
      'ODPS',
      values({ address: '', tableName: '', nullStrsText: 'x', partitions: ['nope', ''] }),
      ['1', '2', '3', '4', '5', '6'],
      { multiNode: true },
    );
    expect(issues).toEqual({
      address: 'required',
      tableName: 'required',
      nullStrs: 'invalid',
      nodeIds: 'tooMany',
      partitions: 'unknownColumn',
    });
    expect(validateRegisterForm('OSS', values(), [], { multiNode: true }).nodeIds).toBe('required');
  });

  it('warns on SQL keyword table names', () => {
    expect(tableKeywordWarnings('MYSQL', values({ tableName: 'select', address: 'order' }))).toEqual({
      tableName: true,
      address: true,
    });
    expect(tableKeywordWarnings('OSS', values({ address: 'order' })).address).toBe(false);
  });
});
