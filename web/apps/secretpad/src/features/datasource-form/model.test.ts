import { describe, expect, it } from 'vitest';
import {
  buildCreateDatasourceRequest,
  buildDataSourceInfo,
  datasourceDeleteBlock,
  datasourceSubmitName,
  emptyDatasourceForm,
  validateDatasourceForm,
} from './model';
import type { DatasourceFormValues } from './model';

const form = (patch: Partial<DatasourceFormValues>): DatasourceFormValues => ({ ...emptyDatasourceForm(patch.type), ...patch });

describe('buildDataSourceInfo', () => {
  it('builds OSS info', () => {
    expect(
      buildDataSourceInfo(
        form({ type: 'OSS', ossEndpoint: ' oss.example.com ', ak: 'ak', sk: 'sk', bucket: 'bkt', prefix: 'p/', virtualhost: true }),
      ),
    ).toEqual({ endpoint: 'oss.example.com', ak: 'ak', sk: 'sk', bucket: 'bkt', prefix: 'p/', virtualhost: true });
  });

  it('builds MYSQL info with host:port endpoint', () => {
    expect(
      buildDataSourceInfo(
        form({ type: 'MYSQL', mysqlHost: '10.0.0.1', mysqlPort: '3307', mysqlUser: 'u', mysqlPassword: 'p', mysqlDatabase: 'db_1' }),
      ),
    ).toEqual({ endpoint: '10.0.0.1:3307', user: 'u', password: 'p', database: 'db_1' });
  });

  it('builds ODPS info', () => {
    expect(
      buildDataSourceInfo(form({ type: 'ODPS', odpsEndpoint: 'e', odpsProject: 'proj', accessId: 'id', accessKey: 'key' })),
    ).toEqual({ endpoint: 'e', project: 'proj', accessId: 'id', accessKey: 'key' });
  });

  it('builds LOCAL and HTTP info', () => {
    expect(buildDataSourceInfo(form({ type: 'LOCAL', localPath: '/data' }))).toEqual({ path: '/data' });
    expect(buildDataSourceInfo(form({ type: 'HTTP' }))).toEqual({});
  });
});

describe('create request', () => {
  it('auto-prefixes ODPS names once', () => {
    expect(datasourceSubmitName(form({ type: 'ODPS', name: 'abc' }))).toBe('ODPS-abc');
    expect(datasourceSubmitName(form({ type: 'ODPS', name: 'ODPS-abc' }))).toBe('ODPS-abc');
    expect(datasourceSubmitName(form({ type: 'OSS', name: 'abc' }))).toBe('abc');
  });

  it('builds the Java CreateDatasourceRequest', () => {
    const req = buildCreateDatasourceRequest(form({ type: 'LOCAL', name: 'ds', localPath: '/p' }), 'alice', ['alice', 'bob']);
    expect(req).toEqual({ ownerId: 'alice', nodeIds: ['alice', 'bob'], type: 'LOCAL', name: 'ds', dataSourceInfo: { path: '/p' } });
  });
});

describe('validateDatasourceForm', () => {
  it('requires OSS fields and checks bucket rules', () => {
    const e = validateDatasourceForm(form({ type: 'OSS', name: 'a b', bucket: 'AB' }), ['alice']);
    expect(e).toMatchObject({ name: 'namePattern', ossEndpoint: 'required', ak: 'required', sk: 'required', bucket: 'bucketLength' });
    expect(validateDatasourceForm(form({ type: 'OSS', bucket: 'Abc' }), ['a']).bucket).toBe('bucketPattern');
  });

  it('checks MYSQL port and database', () => {
    const e = validateDatasourceForm(
      form({ type: 'MYSQL', name: 'm', mysqlHost: 'h', mysqlPort: '99999', mysqlUser: 'u', mysqlPassword: 'p', mysqlDatabase: '1db' }),
      ['a'],
    );
    expect(e).toEqual({ mysqlPort: 'port', mysqlDatabase: 'databasePattern' });
  });

  it('checks node selection limits', () => {
    const ok = form({ type: 'HTTP', name: 'h' });
    expect(validateDatasourceForm(ok, ['a'])).toEqual({});
    expect(validateDatasourceForm(ok, []).nodeIds).toBe('nodesRequired');
    expect(validateDatasourceForm(ok, ['a', '']).nodeIds).toBe('nodesRequired');
    expect(validateDatasourceForm(ok, ['a', 'b', 'c', 'd', 'e', 'f']).nodeIds).toBe('nodesTooMany');
    expect(validateDatasourceForm(ok, ['a', 'a']).nodeIds).toBe('nodesDuplicate');
  });
});

describe('datasourceDeleteBlock', () => {
  it('blocks bound or HTTP datasources', () => {
    expect(datasourceDeleteBlock({ type: 'OSS', relatedDatas: ['t1'] })).toBe('bound');
    expect(datasourceDeleteBlock({ type: 'HTTP' })).toBe('http');
    expect(datasourceDeleteBlock({ type: 'OSS', relatedDatas: [] })).toBeNull();
  });
});
