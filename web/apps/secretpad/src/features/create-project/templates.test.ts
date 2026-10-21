import { describe, expect, it } from 'vitest';
import { buildTemplateConfigs, defaultTableConfigs, templatesForMode } from './templates';

describe('templatesForMode', () => {
  it('filters templates by compute mode; blank fits both', () => {
    const mpc = templatesForMode('MPC').map((t) => t.metadata.key);
    const tee = templatesForMode('TEE').map((t) => t.metadata.key);
    expect(mpc).toContain('blank');
    expect(mpc).toContain('psi');
    expect(mpc).toContain('risk');
    expect(mpc).not.toContain('tee');
    expect(tee).toContain('blank');
    expect(tee).toContain('tee');
    expect(tee).not.toContain('risk');
    expect(tee).not.toContain('psi');
  });
});

describe('buildTemplateConfigs', () => {
  it('maps the first two participants to receiver/sender for two-table templates', () => {
    const cfg = buildTemplateConfigs('psi', [
      { nodeId: 'alice', datatableId: 't1' },
      { nodeId: 'bob', datatableId: 't2' },
    ]);
    expect(cfg).toMatchObject({
      receiverNodeId: 'alice',
      senderNodeId: 'bob',
      receiverTableId: 't1',
      senderTableId: 't2',
      receiverKey: 'id1',
      senderKey: 'id2',
    });
  });
  it('adds label/prediction defaults for risk', () => {
    const cfg = buildTemplateConfigs('risk', [{ nodeId: 'alice' }, { nodeId: 'bob' }]);
    expect(cfg.labelSelects).toEqual({ s: 'y' });
    expect(cfg.pred).toEqual({ s: 'pred' });
  });
  it('uses the first participant with a table for single-table templates', () => {
    const cfg = buildTemplateConfigs('kAnonymity', [{ nodeId: 'n1' }, { nodeId: 'n2', datatableId: 'tb' }]);
    expect(cfg).toMatchObject({ nodeId: 'n2', tableId: 'tb', qiCols: [], saCols: [] });
  });
  it('returns empty configs for blank / unknown templates', () => {
    expect(buildTemplateConfigs('blank', [])).toEqual({});
    expect(buildTemplateConfigs('nope', [])).toEqual({});
  });
});

describe('defaultTableConfigs', () => {
  it('only configures embedded demo nodes for two-table templates', () => {
    expect(defaultTableConfigs('alice', 'psi')?.[0]).toMatchObject({ colName: 'id1', isAssociateKey: true });
    expect(defaultTableConfigs('bob', 'risk')?.[0]).toMatchObject({ colName: 'id2' });
    expect(defaultTableConfigs('carol', 'psi')).toBeUndefined();
    expect(defaultTableConfigs('alice', 'blank')).toBeUndefined();
    expect(defaultTableConfigs('alice', 'dataClassification')).toBeUndefined();
  });
});
