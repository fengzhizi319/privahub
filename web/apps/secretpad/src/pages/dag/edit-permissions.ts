/**
 * DAG 页细粒度编辑权限（旧版 P2P `ProjectEditService.canEdit` +
 * `PipelineService.changePipelineCanEdit`）。
 *
 * 旧版 12 个开关按作用范围归并为两级：
 * - `graph`：当前训练流画布级写操作——右侧配置表单、右键菜单、快捷键、工具栏运行/停止、
 *   拖入算子与数据表、训练流重命名/删除/复制（pipelineEdit + 非我方锁定）、运行记录停止任务；
 * - `project`：项目级写操作——新建训练流（含模板/快速配置之外的新建）、提交模型、
 *   全局配置保存、周期任务、数据表树「去添加数据」。
 *
 * 规则（旧版只在 P2P 模式生效，CENTER 默认全部可编辑）：
 * 1. 项目不在列表中或已归档 → 全部禁用（changeCanEditTrue）；
 * 2. 训练流非我方创建 → 画布禁用，项目级操作保留，运行提示「非我方节点创建，仅可查看」；
 * 3. 其它 → 全部可用。
 * 新版额外：待审批（REVIEWING）项目全部只读；无 DAG 权限的平台（EDGE）全部只读。
 */
import { Platform } from '../../shared/lib/platform';

export type DagReadOnlyReason = 'noPermission' | 'projectMissing' | 'archived' | 'reviewing' | 'foreignGraph';

export interface DagEditPermissions {
  /** 项目级写操作。 */
  project: boolean;
  /** 当前训练流画布级写操作（隐含 project）。 */
  graph: boolean;
  /** 只读原因（全部可编辑时为 null）。 */
  reason: DagReadOnlyReason | null;
}

export interface DagEditInput {
  platformType: Platform;
  /** 当前登录节点（userInfo.ownerId）。 */
  ownerId: string;
  /** 当前项目（列表项或详情）；undefined 表示未找到。 */
  project?: { status?: string } | null;
  /** 项目列表是否已加载（未加载时不因「找不到项目」判只读）。 */
  projectsLoaded: boolean;
  /** 当前训练流的创建节点。 */
  graphOwnerId?: string;
}

const EDITABLE_PLATFORMS = [Platform.CENTER, Platform.TEST, Platform.AUTONOMY, Platform.P2P];

const denied = (reason: DagReadOnlyReason): DagEditPermissions => ({ project: false, graph: false, reason });

export function dagEditPermissions({ platformType, ownerId, project, projectsLoaded, graphOwnerId }: DagEditInput): DagEditPermissions {
  if (!EDITABLE_PLATFORMS.includes(platformType)) return denied('noPermission');
  const p2p = platformType === Platform.AUTONOMY || platformType === Platform.P2P;
  if (p2p && projectsLoaded && !project) return denied('projectMissing');
  const status = String(project?.status || '').toUpperCase();
  if (status === 'ARCHIVED') return denied('archived');
  if (status === 'REVIEWING') return denied('reviewing');
  if (p2p && graphOwnerId && ownerId && graphOwnerId !== ownerId) return { project: true, graph: false, reason: 'foreignGraph' };
  return { project: true, graph: true, reason: null };
}
