/**
 * Java-contract extension APIs: scheduled (periodic tasks).
 *
 * Mirrors `ScheduledController.ts` of the legacy Java SecretPad frontend.
 * Date formats (see legacy `create-periodic-task.view.tsx`):
 * - `cron.startTime` / `cron.endTime`: `YYYY-MM-DD HH:mm:ss`
 * - `cron.scheduleTime`: `HH:mm:ss`
 * - `cron.scheduleCycle`: `D` | `W` | `M`
 * - `cron.scheduleDate`: `''` for D; comma-joined weekdays `1..7` (7 = Sunday) for W;
 *   comma-joined month days `1..31` plus `end` (last day of month) for M.
 */
import { javaPost, toPage, type JavaPage } from './core';

export type ScheduleCycleJava = 'D' | 'W' | 'M';

export interface ScheduledCronJava {
  startTime: string;
  endTime: string;
  scheduleCycle: ScheduleCycleJava;
  scheduleDate: string;
  scheduleTime: string;
}

export interface ScheduledGraphCreateRequestJava {
  scheduleId: string;
  scheduleDesc?: string;
  cron: ScheduledCronJava;
  projectId: string;
  graphId: string;
  nodes: string[];
}

export interface PageScheduledRequestJava {
  page?: number;
  size?: number;
  sort?: Record<string, string>;
  /** scheduleId fuzzy search */
  search?: string;
  /** '' | 'UP' | 'DOWN' */
  status?: string;
  projectId?: string;
}

export interface PageScheduledVOJava {
  scheduleId?: string;
  scheduleDesc?: string;
  /** `UP` (生效中) | `DOWN` (已下线) */
  scheduleStats?: string;
  creator?: string;
  createTime?: string;
  taskRunning?: boolean;
  /** P2P/AUTONOMY: owning institution id */
  owner?: string;
  ownerName?: string;
}

export interface TaskPageScheduledRequestJava {
  scheduleId: string;
  page?: number;
  size?: number;
  sort?: Record<string, string>;
  search?: string;
}

export interface TaskPageScheduledVOJava {
  scheduleTaskId?: string;
  scheduleTaskExpectStartTime?: string;
  scheduleTaskStartTime?: string;
  scheduleTaskEndTime?: string;
  /** SUCCEED | FAILED | RUNNING | TO_BE_RUN | STOPPED | STOPPING */
  scheduleTaskStatus?: string;
  /** Already fully re-run (legacy disables a second re-run) */
  allReRun?: boolean;
}

/** Legacy rerun `type`: `'0'` = rerun all, `'1'` = rerun failed part only. */
export const RERUN_TYPE_ALL = '0';
export const RERUN_TYPE_FAILED = '1';
export type RerunTypeJava = typeof RERUN_TYPE_ALL | typeof RERUN_TYPE_FAILED;

export interface ScheduledGraphNodeVOJava {
  graphNodeId?: string;
  codeName?: string;
  label?: string;
  status?: string;
  jobId?: string;
  taskId?: string;
  x?: number;
  y?: number;
  inputs?: string[];
  outputs?: string[];
  parties?: { nodeId?: string; nodeName?: string }[];
}

/** Java `ProjectJobVO` (scheduled/info, scheduled/task/info). */
export interface ScheduledJobVOJava {
  jobId?: string;
  status?: string;
  errMsg?: string;
  gmtCreate?: string;
  gmtModified?: string;
  gmtFinished?: string;
  finished?: boolean;
  graph?: {
    projectId?: string;
    graphId?: string;
    name?: string;
    nodes?: ScheduledGraphNodeVOJava[];
    edges?: unknown;
  };
}

/** Java `ProjectJobSummaryVO`. */
export interface ProjectJobSummaryVOJava {
  jobId?: string;
  name?: string;
  status?: string;
  errMsg?: string;
  gmtCreate?: string;
  gmtModified?: string;
  gmtFinished?: string;
  tableCount?: number;
  modelCount?: number;
  ruleCount?: number;
  reportCount?: number;
  finishedTaskCount?: number;
  taskCount?: number;
}

/**
 * Normalise a Java `PageResponse` (`{pageTotal (page count), pageSize, data}`),
 * falling back to `{list,total}` shapes.
 */
export function toJobPage<T>(payload: unknown, pageSize: number): JavaPage<T> {
  const page = toPage<T>(payload, ['data', 'list']);
  const p = (payload || {}) as Record<string, unknown>;
  // Go returns an exact `total`; toPage already prefers it. Java PageResponse only has
  // pageTotal (page count): a single page means the list *is* the total, otherwise the
  // best estimate is pageTotal × pageSize (enough to render the right number of pages).
  if (p.total === undefined && p.totalCount === undefined && typeof p.pageTotal === 'number') {
    if (p.pageTotal <= 1) return { list: page.list, total: page.list.length };
    const size = Number(p.pageSize) || pageSize;
    return { list: page.list, total: Math.max(page.list.length, p.pageTotal * size) };
  }
  return page;
}

/** POST scheduled/id → new schedule id for a graph. */
export async function getScheduledIdJava(req: { projectId: string; graphId: string }): Promise<string> {
  return (await javaPost<string>('scheduled/id', req)) || '';
}

/** POST scheduled/graph/once/success → whether the graph ran successfully once. */
export async function getScheduledOnceSuccessJava(req: { projectId: string; graphId: string }): Promise<boolean> {
  return !!(await javaPost<boolean>('scheduled/graph/once/success', req));
}

export async function createScheduledJava(req: ScheduledGraphCreateRequestJava): Promise<void> {
  await javaPost('scheduled/graph/create', req);
}

export async function pageScheduledJava(req: PageScheduledRequestJava): Promise<JavaPage<PageScheduledVOJava>> {
  return toPage<PageScheduledVOJava>(await javaPost('scheduled/page', req), ['list']);
}

export async function offlineScheduledJava(scheduleId: string): Promise<void> {
  await javaPost('scheduled/offline', { scheduleId });
}

export async function deleteScheduledJava(scheduleId: string): Promise<void> {
  await javaPost('scheduled/del', { scheduleId });
}

/** POST scheduled/info → graph/job snapshot of the periodic task. */
export async function getScheduledInfoJava(scheduleId: string): Promise<ScheduledJobVOJava> {
  return (await javaPost<ScheduledJobVOJava>('scheduled/info', { scheduleId })) || {};
}

export async function pageScheduledTasksJava(
  req: TaskPageScheduledRequestJava,
): Promise<JavaPage<TaskPageScheduledVOJava>> {
  return toPage<TaskPageScheduledVOJava>(await javaPost('scheduled/task/page', req), ['list']);
}

export async function rerunScheduledTaskJava(req: {
  scheduleId: string;
  scheduleTaskId: string;
  type: RerunTypeJava;
}): Promise<void> {
  await javaPost('scheduled/task/rerun', req);
}

export async function stopScheduledTaskJava(req: { scheduleId: string; scheduleTaskId: string }): Promise<void> {
  await javaPost('scheduled/task/stop', req);
}

/** POST scheduled/task/info → graph/job snapshot of one sub task. */
export async function getScheduledTaskInfoJava(req: {
  scheduleId: string;
  scheduleTaskId: string;
}): Promise<ScheduledJobVOJava> {
  return (await javaPost<ScheduledJobVOJava>('scheduled/task/info', req)) || {};
}

/** POST scheduled/job/list → run records of a sub task (Java PageResponse). */
export async function listScheduledJobsJava(req: {
  projectId: string;
  graphId: string;
  scheduleTaskId?: string;
  searchKey?: string;
  status?: string;
  startTime?: string;
  endTime?: string;
  pageNum: number;
  pageSize: number;
}): Promise<JavaPage<ProjectJobSummaryVOJava>> {
  return toJobPage<ProjectJobSummaryVOJava>(await javaPost('scheduled/job/list', req), req.pageSize);
}

/** POST project/job/list (optionally filtered by graph) → Java PageResponse of job summaries. */
export async function listProjectJobSummariesJava(req: {
  projectId: string;
  graphId?: string;
  pageNum: number;
  pageSize: number;
}): Promise<JavaPage<ProjectJobSummaryVOJava>> {
  return toJobPage<ProjectJobSummaryVOJava>(await javaPost('project/job/list', req), req.pageSize);
}
