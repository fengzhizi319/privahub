/**
 * 统一的图节点 / 任务状态常量。
 *
 * 后端契约（Java SecretPad typings.d.ts `GraphNodeTaskStatus` / `GraphJobStatus`）：
 *   STAGING | INITIALIZED | RUNNING | STOPPED | SUCCEED | FAILED
 * 过去前端各处混用 `SUCCEED` / `SUCCEEDED` / `SUCCESS`，这里集中定义并提供容错归一化，
 * 所有页面（DAG、运行记录、任务详情）都应只使用本文件导出的常量。
 */

export const NODE_STATUS = {
  STAGING: 'STAGING',
  INITIALIZED: 'INITIALIZED',
  RUNNING: 'RUNNING',
  SUCCEED: 'SUCCEED',
  FAILED: 'FAILED',
  STOPPED: 'STOPPED',
} as const;

export type BackendNodeStatus = (typeof NODE_STATUS)[keyof typeof NODE_STATUS];

/** 任务（Job）状态与节点状态共用同一枚举。 */
export const JOB_STATUS = NODE_STATUS;
export type BackendJobStatus = BackendNodeStatus;

/** 画布内部展示状态。 */
export type DAGNodeStatus =
  | 'Ready'
  | 'Pending'
  | 'Running'
  | 'Success'
  | 'Failed'
  | 'Staging'
  | 'Stopped';

/**
 * 把后端各种历史写法归一为标准枚举：
 * - SUCCEEDED / SUCCESS / SUCCEED → SUCCEED
 * - IDLE / PENDING_CONFIG / '' → STAGING
 * - PENDING / INITIALIZING → INITIALIZED
 * - STOP / CANCELED → STOPPED
 */
export function normalizeStatus(status?: string | null): BackendNodeStatus | undefined {
  if (!status) return undefined;
  const s = String(status).trim().toUpperCase();
  switch (s) {
    case 'SUCCEED':
    case 'SUCCEEDED':
    case 'SUCCESS':
      return NODE_STATUS.SUCCEED;
    case 'FAILED':
    case 'FAIL':
    case 'ERROR':
      return NODE_STATUS.FAILED;
    case 'RUNNING':
      return NODE_STATUS.RUNNING;
    case 'INITIALIZED':
    case 'PENDING':
    case 'INITIALIZING':
      return NODE_STATUS.INITIALIZED;
    case 'STOPPED':
    case 'STOP':
    case 'CANCELED':
    case 'CANCELLED':
      return NODE_STATUS.STOPPED;
    case 'STAGING':
    case 'IDLE':
      return NODE_STATUS.STAGING;
    default:
      return undefined;
  }
}

/** 节点是否处于“执行中”（需要继续轮询）。 */
export function isActiveStatus(status?: string | null): boolean {
  const s = normalizeStatus(status);
  return s === NODE_STATUS.RUNNING || s === NODE_STATUS.INITIALIZED;
}

/** 节点是否处于终态。 */
export function isTerminalStatus(status?: string | null): boolean {
  const s = normalizeStatus(status);
  return s === NODE_STATUS.SUCCEED || s === NODE_STATUS.FAILED || s === NODE_STATUS.STOPPED;
}

/** 后端状态 → 画布状态。 */
export function toCanvasStatus(status?: string | null): DAGNodeStatus {
  switch (normalizeStatus(status)) {
    case NODE_STATUS.RUNNING:
      return 'Running';
    case NODE_STATUS.INITIALIZED:
      return 'Pending';
    case NODE_STATUS.SUCCEED:
      return 'Success';
    case NODE_STATUS.FAILED:
      return 'Failed';
    case NODE_STATUS.STOPPED:
      return 'Stopped';
    default:
      return 'Ready';
  }
}

/** Badge 语义色。 */
export function statusBadge(status?: string | null): 'success' | 'processing' | 'error' | 'warning' | 'default' {
  switch (normalizeStatus(status)) {
    case NODE_STATUS.SUCCEED:
      return 'success';
    case NODE_STATUS.RUNNING:
    case NODE_STATUS.INITIALIZED:
      return 'processing';
    case NODE_STATUS.FAILED:
      return 'error';
    case NODE_STATUS.STOPPED:
      return 'warning';
    default:
      return 'default';
  }
}
