/**
 * Structured periodic-task schedule: pure validation + payload builder.
 *
 * Ported from the legacy `periodic-task-drawer` (create-periodic-task.view.tsx,
 * utils.ts, create-periodic-task-service.ts):
 * - scheduleCycle D/W/M; W days are `1..7` (7 = Sunday); M days are `1..31`
 *   plus `-1` (last day of month, sent as `end`); at most 5 days.
 * - schedule range must be > 24h and its end must be after "now".
 * - there must be at least one schedulable moment inside the range.
 * - description ≤ 200 characters.
 * - payload: startTime/endTime `YYYY-MM-DD HH:mm:ss`, scheduleTime `HH:mm:ss`.
 */
import type { ScheduleCycleJava, ScheduledGraphCreateRequestJava } from '@secretpad/api-client';

export type ScheduleCycle = ScheduleCycleJava;

export const SCHEDULE_MAX_DAYS = 5;
export const SCHEDULE_DESC_MAX = 200;
export const SCHEDULE_MIN_RANGE_MS = 24 * 60 * 60 * 1000;
/** Legacy month option for "last day of month". */
export const LAST_DAY_OF_MONTH = '-1';

export const WEEK_DAY_KEYS = ['1', '2', '3', '4', '5', '6', '7'];
export const MONTH_DAY_KEYS = [...Array.from({ length: 31 }, (_, i) => String(i + 1)), LAST_DAY_OF_MONTH];

export interface ScheduleFormValues {
  scheduleId: string;
  scheduleDesc: string;
  /** `YYYY-MM-DDTHH:mm` (datetime-local) or `YYYY-MM-DD HH:mm[:ss]` */
  startTime: string;
  endTime: string;
  cycle: ScheduleCycle | '';
  /** W: '1'..'7'; M: '1'..'31' | '-1'; D: [] */
  days: string[];
  /** `HH:mm` */
  time: string;
}

export type ScheduleField = keyof ScheduleFormValues | 'form';

/** Field → i18n key of the error message. */
export type ScheduleErrors = Partial<Record<ScheduleField, string>>;

const pad = (n: number) => String(n).padStart(2, '0');

export function formatDateTime(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(
    d.getMinutes(),
  )}:${pad(d.getSeconds())}`;
}

/** `YYYY-MM-DDTHH:mm` value for `<input type="datetime-local">`. */
export function toDateTimeLocal(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** Parse local date-time strings (`T` or space separated, optional seconds). */
export function parseDateTime(value: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2})(?::(\d{2}))?$/.exec((value || '').trim());
  if (!m) return null;
  const [, y, mo, d, h, mi, s] = m;
  const date = new Date(Number(y), Number(mo) - 1, Number(d), Number(h), Number(mi), Number(s || 0));
  return Number.isNaN(date.getTime()) ? null : date;
}

export function isValidTime(value: string): boolean {
  return /^([01]\d|2[0-3]):[0-5]\d(:[0-5]\d)?$/.test((value || '').trim());
}

/** `HH:mm` → `HH:mm:ss`. */
export function toScheduleTime(value: string): string {
  const v = (value || '').trim();
  return v.length === 5 ? `${v}:00` : v;
}

const sortDays = (days: string[]) =>
  [...new Set(days)].sort((a, b) => {
    const na = Number(a) === -1 ? 99 : Number(a);
    const nb = Number(b) === -1 ? 99 : Number(b);
    return na - nb;
  });

/** Legacy `scheduleDate` string: D → '', W → '1,3,7', M → '1,15,end'. */
export function buildScheduleDate(cycle: ScheduleCycle, days: string[]): string {
  if (cycle === 'D') return '';
  return sortDays(days)
    .map((d) => (d === LAST_DAY_OF_MONTH ? 'end' : d))
    .join(',');
}

const ymd = (d: Date) => `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}`;

/**
 * Dates (`YYYYMMDD`) within [start, end] on which the schedule would fire.
 * W days use legacy keys `1..7` (7 = Sunday); M day `-1` = last day of month,
 * days beyond a month's length fall back to its last day.
 */
export function getScheduledDatesWithinRange(
  start: Date,
  end: Date,
  cycle: ScheduleCycle,
  days: string[],
  time: string,
): string[] {
  const [hh, mm, ss] = toScheduleTime(time).split(':').map(Number);
  const startMs = start.getTime();
  const endMs = end.getTime();
  const result = new Set<string>();
  const cursor = new Date(start.getFullYear(), start.getMonth(), start.getDate());
  let guard = 0;
  while (cursor.getTime() <= endMs && guard < 366 * 20) {
    guard += 1;
    const candidates: Date[] = [];
    if (cycle === 'D') {
      candidates.push(new Date(cursor));
    } else if (cycle === 'W') {
      const weekday = cursor.getDay() === 0 ? 7 : cursor.getDay();
      if (days.includes(String(weekday))) candidates.push(new Date(cursor));
    } else if (cycle === 'M') {
      const lastDay = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 0).getDate();
      const target = days.map((d) => (d === LAST_DAY_OF_MONTH ? lastDay : Math.min(Number(d), lastDay)));
      if (target.includes(cursor.getDate())) candidates.push(new Date(cursor));
    }
    for (const c of candidates) {
      const moment = new Date(c.getFullYear(), c.getMonth(), c.getDate(), hh || 0, mm || 0, ss || 0).getTime();
      if (moment >= startMs && moment <= endMs) result.add(ymd(c));
    }
    cursor.setDate(cursor.getDate() + 1);
  }
  return [...result];
}

/** Validate the form; returns i18n error keys per field (empty object = valid). */
export function validateSchedule(values: ScheduleFormValues, now: Date = new Date()): ScheduleErrors {
  const errors: ScheduleErrors = {};
  if (!values.scheduleId) errors.scheduleId = 'scheduleForm.errors.scheduleId';
  if ((values.scheduleDesc || '').length > SCHEDULE_DESC_MAX) errors.scheduleDesc = 'scheduleForm.errors.descTooLong';

  const start = parseDateTime(values.startTime);
  const end = parseDateTime(values.endTime);
  if (!start || !end) {
    errors.startTime = 'scheduleForm.errors.rangeRequired';
  } else if (end.getTime() - start.getTime() < SCHEDULE_MIN_RANGE_MS) {
    errors.startTime = 'scheduleForm.errors.rangeTooShort';
  } else if (end.getTime() < now.getTime()) {
    errors.endTime = 'scheduleForm.errors.endBeforeNow';
  }

  if (!values.cycle) {
    errors.cycle = 'scheduleForm.errors.cycleRequired';
  } else if (values.cycle !== 'D') {
    const allowed = values.cycle === 'W' ? WEEK_DAY_KEYS : MONTH_DAY_KEYS;
    const days = values.days || [];
    if (days.length === 0) errors.days = 'scheduleForm.errors.daysRequired';
    else if (days.length > SCHEDULE_MAX_DAYS) errors.days = 'scheduleForm.errors.daysTooMany';
    else if (days.some((d) => !allowed.includes(d))) errors.days = 'scheduleForm.errors.daysInvalid';
  }

  if (!values.time) errors.time = 'scheduleForm.errors.timeRequired';
  else if (!isValidTime(values.time)) errors.time = 'scheduleForm.errors.timeInvalid';

  if (Object.keys(errors).length === 0 && start && end && values.cycle) {
    const dates = getScheduledDatesWithinRange(start, end, values.cycle, values.days || [], values.time);
    if (dates.length === 0) errors.form = 'scheduleForm.errors.noSchedulableDate';
  }
  return errors;
}

export function isScheduleValid(errors: ScheduleErrors): boolean {
  return Object.keys(errors).length === 0;
}

/** Build the Java `ScheduledGraphCreateRequest` (call only after validation passes). */
export function buildSchedulePayload(
  values: ScheduleFormValues,
  ctx: { projectId: string; graphId: string; nodes: string[] },
): ScheduledGraphCreateRequestJava {
  const start = parseDateTime(values.startTime);
  const end = parseDateTime(values.endTime);
  if (!start || !end || !values.cycle) throw new Error('Invalid schedule');
  const desc = (values.scheduleDesc || '').trim();
  return {
    scheduleId: values.scheduleId,
    ...(desc ? { scheduleDesc: desc } : {}),
    cron: {
      startTime: formatDateTime(start),
      endTime: formatDateTime(end),
      scheduleCycle: values.cycle,
      scheduleDate: buildScheduleDate(values.cycle, values.days || []),
      scheduleTime: toScheduleTime(values.time),
    },
    projectId: ctx.projectId,
    graphId: ctx.graphId,
    nodes: ctx.nodes,
  };
}

/** Initial values: start = now + 2 min (legacy default), everything else empty. */
export function initialScheduleValues(scheduleId = '', now: Date = new Date()): ScheduleFormValues {
  return {
    scheduleId,
    scheduleDesc: '',
    startTime: toDateTimeLocal(new Date(now.getTime() + 2 * 60 * 1000)),
    endTime: '',
    cycle: '',
    days: [],
    time: '',
  };
}
