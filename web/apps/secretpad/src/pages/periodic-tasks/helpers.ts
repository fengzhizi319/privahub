/**
 * Pure helpers for the periodic task list / sub-task history (legacy
 * `periodic-task-list/task.service.tsx` and `periodic-child-task-list`).
 */
import {
  RERUN_TYPE_ALL,
  RERUN_TYPE_FAILED,
  type PageScheduledVOJava,
  type RerunTypeJava,
  type TaskPageScheduledVOJava,
} from '@secretpad/api-client';
import { NODE_STATUS, normalizeStatus } from '@secretpad/dag-next';

export type BadgeStatus = 'success' | 'processing' | 'warning' | 'error' | 'default';

/** Periodic task status (`scheduleStats`). */
export const SCHEDULE_STATUS = { UP: 'UP', DOWN: 'DOWN' } as const;

/** Sub task status (`scheduleTaskStatus`). */
export const SUB_TASK_STATUS = {
  SUCCEED: NODE_STATUS.SUCCEED,
  FAILED: NODE_STATUS.FAILED,
  RUNNING: NODE_STATUS.RUNNING,
  TO_BE_RUN: 'TO_BE_RUN',
  STOPPED: NODE_STATUS.STOPPED,
  STOPPING: 'STOPPING',
} as const;

export function scheduleStatusBadge(status?: string): BadgeStatus {
  return status === SCHEDULE_STATUS.UP ? 'success' : 'default';
}

/** Job / graph node status badge. */
export function jobStatusBadge(status?: string): BadgeStatus {
  const upper = (status || '').toUpperCase();
  // 周期子任务特有状态。
  if (upper === SUB_TASK_STATUS.TO_BE_RUN || upper === 'PENDING') return 'warning';
  if (upper === SUB_TASK_STATUS.STOPPING) return 'processing';
  // 其余走统一的 job / 节点状态（dag-next/status）。
  switch (normalizeStatus(status)) {
    case NODE_STATUS.SUCCEED:
      return 'success';
    case NODE_STATUS.FAILED:
    case NODE_STATUS.STOPPED:
      return 'error';
    case NODE_STATUS.RUNNING:
    case NODE_STATUS.INITIALIZED:
    case NODE_STATUS.STAGING:
      return 'processing';
    default:
      return 'default';
  }
}


export function subTaskStatusBadge(status?: string): BadgeStatus {
  switch (status) {
    case SUB_TASK_STATUS.SUCCEED:
      return 'success';
    case SUB_TASK_STATUS.FAILED:
    case SUB_TASK_STATUS.STOPPED:
      return 'error';
    case SUB_TASK_STATUS.RUNNING:
    case SUB_TASK_STATUS.STOPPING:
      return 'processing';
    case SUB_TASK_STATUS.TO_BE_RUN:
      return 'warning';
    default:
      return 'default';
  }
}

/** Legacy: in AUTONOMY/P2P only the owning institution may operate; elsewhere everyone. */
export function canOperateSchedule(task: PageScheduledVOJava, ctx: { isP2p: boolean; ownerId: string }): boolean {
  if (!ctx.isP2p) return true;
  return !!task.owner && task.owner === ctx.ownerId;
}

export type ScheduleAction = 'offline' | 'offlineDisabled' | 'delete';

/** UP → offline (disabled while a sub task runs); DOWN → delete. */
export function scheduleAction(task: PageScheduledVOJava): ScheduleAction {
  if (task.scheduleStats === SCHEDULE_STATUS.UP) return task.taskRunning ? 'offlineDisabled' : 'offline';
  return 'delete';
}

/** Whether the sub-task list should keep polling (legacy: every 10s). */
export function shouldPollSubTasks(list: TaskPageScheduledVOJava[]): boolean {
  return list.some((i) =>
    [SUB_TASK_STATUS.RUNNING, SUB_TASK_STATUS.TO_BE_RUN, SUB_TASK_STATUS.STOPPING].includes(
      i.scheduleTaskStatus as never,
    ),
  );
}

export interface RerunOption {
  type: RerunTypeJava;
  /** i18n key of the button label */
  labelKey: string;
  disabled: boolean;
}

export interface SubTaskActions {
  stop: boolean;
  rerun: RerunOption[];
  /** i18n key of the rerun confirmation text */
  confirmKey?: string;
}

const THIRTY_DAYS_MS = 30 * 24 * 60 * 60 * 1000;

function parseTime(v?: string): number {
  if (!v) return NaN;
  if (/^\d+$/.test(v)) return Number(v);
  return new Date(v.replace(' ', 'T')).getTime();
}

/**
 * Legacy rerun rules (`renderOptionsTitle` + child-task.view):
 * - RUNNING → stop only.
 * - FAILED/STOPPED within 30 days of its start → "rerun failed part" (type 1) or "rerun all" (type 0).
 * - SUCCEED, or FAILED/STOPPED older than 30 days → "rerun" (type 0).
 * - "rerun all" is disabled once the task was already fully re-run (`allReRun`).
 */
export function subTaskActions(task: TaskPageScheduledVOJava, now: number = Date.now()): SubTaskActions {
  const status = task.scheduleTaskStatus;
  if (status === SUB_TASK_STATUS.RUNNING) return { stop: true, rerun: [] };
  const failed = status === SUB_TASK_STATUS.FAILED || status === SUB_TASK_STATUS.STOPPED;
  if (!failed && status !== SUB_TASK_STATUS.SUCCEED) return { stop: false, rerun: [] };
  const started = parseTime(task.scheduleTaskStartTime);
  const overThirtyDays = Number.isNaN(started) ? true : now - started > THIRTY_DAYS_MS;
  const all: RerunOption = { type: RERUN_TYPE_ALL, labelKey: 'periodicTasks.rerunAll', disabled: !!task.allReRun };
  if (failed && !overThirtyDays) {
    return {
      stop: false,
      rerun: [{ type: RERUN_TYPE_FAILED, labelKey: 'periodicTasks.rerunFailed', disabled: false }, all],
      confirmKey: 'periodicTasks.rerunFailedConfirm',
    };
  }
  return {
    stop: false,
    rerun: [{ ...all, labelKey: 'periodicTasks.rerun' }],
    confirmKey: failed ? 'periodicTasks.rerunOverwritePartial' : 'periodicTasks.rerunOverwrite',
  };
}
