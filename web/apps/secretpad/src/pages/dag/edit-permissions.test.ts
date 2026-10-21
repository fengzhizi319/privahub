import { describe, expect, it } from 'vitest';
import { Platform } from '../../shared/lib/platform';
import { dagEditPermissions } from './edit-permissions';

const base = { ownerId: 'alice', project: { status: 'APPROVED' }, projectsLoaded: true };

describe('dagEditPermissions（旧版 ProjectEditService.canEdit）', () => {
  it('CENTER / TEST：默认全部可编辑，不看训练流创建方', () => {
    expect(dagEditPermissions({ ...base, platformType: Platform.CENTER, graphOwnerId: 'bob' })).toEqual({ project: true, graph: true, reason: null });
    expect(dagEditPermissions({ ...base, platformType: Platform.TEST })).toEqual({ project: true, graph: true, reason: null });
  });

  it('EDGE 无训练流编辑权限', () => {
    expect(dagEditPermissions({ ...base, platformType: Platform.EDGE })).toEqual({ project: false, graph: false, reason: 'noPermission' });
  });

  it('P2P：项目不在列表中 → 全部禁用（列表未加载时不判定）', () => {
    expect(dagEditPermissions({ ...base, platformType: Platform.AUTONOMY, project: null })).toMatchObject({ project: false, graph: false, reason: 'projectMissing' });
    expect(dagEditPermissions({ ...base, platformType: Platform.AUTONOMY, project: null, projectsLoaded: false }).graph).toBe(true);
  });

  it('归档 / 待审批项目全部只读', () => {
    expect(dagEditPermissions({ ...base, platformType: Platform.P2P, project: { status: 'archived' } })).toMatchObject({ project: false, graph: false, reason: 'archived' });
    expect(dagEditPermissions({ ...base, platformType: Platform.CENTER, project: { status: 'REVIEWING' } }).reason).toBe('reviewing');
  });

  it('P2P：非我方训练流 → 画布只读，项目级操作（新建训练流、提交模型、全局配置）保留', () => {
    expect(dagEditPermissions({ ...base, platformType: Platform.AUTONOMY, graphOwnerId: 'bob' })).toEqual({ project: true, graph: false, reason: 'foreignGraph' });
    expect(dagEditPermissions({ ...base, platformType: Platform.AUTONOMY, graphOwnerId: 'alice' })).toEqual({ project: true, graph: true, reason: null });
    // 训练流不存在（无 ownerId）：画布本身无内容，项目级操作可用。
    expect(dagEditPermissions({ ...base, platformType: Platform.P2P })).toEqual({ project: true, graph: true, reason: null });
  });
});
