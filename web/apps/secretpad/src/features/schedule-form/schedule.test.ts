import { describe, expect, it } from 'vitest';
import {
  buildScheduleDate,
  buildSchedulePayload,
  getScheduledDatesWithinRange,
  initialScheduleValues,
  isScheduleValid,
  parseDateTime,
  validateSchedule,
  type ScheduleFormValues,
} from './schedule';

const now = new Date(2026, 0, 10, 12, 0, 0); // 2026-01-10 (Saturday) 12:00

const base = (patch: Partial<ScheduleFormValues> = {}): ScheduleFormValues => ({
  scheduleId: 'sch-1',
  scheduleDesc: '',
  startTime: '2026-01-10T12:05',
  endTime: '2026-01-20T12:05',
  cycle: 'D',
  days: [],
  time: '02:00',
  ...patch,
});

describe('schedule validation', () => {
  it('accepts a valid daily schedule', () => {
    expect(validateSchedule(base(), now)).toEqual({});
  });

  it('requires the schedule id (scheduled/id prefetch)', () => {
    expect(validateSchedule(base({ scheduleId: '' }), now).scheduleId).toBe('scheduleForm.errors.scheduleId');
  });

  it('limits the description to 200 characters', () => {
    expect(validateSchedule(base({ scheduleDesc: 'x'.repeat(201) }), now).scheduleDesc).toBeDefined();
    expect(validateSchedule(base({ scheduleDesc: 'x'.repeat(200) }), now).scheduleDesc).toBeUndefined();
  });

  it('requires the range to be at least 24 hours', () => {
    const e = validateSchedule(base({ endTime: '2026-01-11T12:04' }), now);
    expect(e.startTime).toBe('scheduleForm.errors.rangeTooShort');
    expect(validateSchedule(base({ endTime: '2026-01-11T12:05' }), now).startTime).toBeUndefined();
  });

  it('requires the range end to be after now', () => {
    const e = validateSchedule(base({ startTime: '2025-12-01T00:00', endTime: '2026-01-05T00:00' }), now);
    expect(e.endTime).toBe('scheduleForm.errors.endBeforeNow');
  });

  it('requires days for weekly / monthly cycles and caps them at 5', () => {
    expect(validateSchedule(base({ cycle: 'W', days: [] }), now).days).toBe('scheduleForm.errors.daysRequired');
    expect(validateSchedule(base({ cycle: 'M', days: ['1', '2', '3', '4', '5', '6'] }), now).days).toBe(
      'scheduleForm.errors.daysTooMany',
    );
    expect(validateSchedule(base({ cycle: 'W', days: ['8'] }), now).days).toBe('scheduleForm.errors.daysInvalid');
    expect(validateSchedule(base({ cycle: '' }), now).cycle).toBe('scheduleForm.errors.cycleRequired');
  });

  it('validates the time of day', () => {
    expect(validateSchedule(base({ time: '' }), now).time).toBe('scheduleForm.errors.timeRequired');
    expect(validateSchedule(base({ time: '25:00' }), now).time).toBe('scheduleForm.errors.timeInvalid');
  });

  it('rejects ranges without any schedulable date', () => {
    // 2026-01-12 (Mon) 13:00 → 2026-01-13 (Tue) 13:30: no Sunday inside
    const e = validateSchedule(
      base({ startTime: '2026-01-12T13:00', endTime: '2026-01-13T13:30', cycle: 'W', days: ['7'], time: '10:00' }),
      now,
    );
    expect(e.form).toBe('scheduleForm.errors.noSchedulableDate');
    expect(isScheduleValid(e)).toBe(false);
  });
});

describe('getScheduledDatesWithinRange', () => {
  it('handles weekly keys with 7 = Sunday', () => {
    const dates = getScheduledDatesWithinRange(
      new Date(2026, 0, 10, 0, 0),
      new Date(2026, 0, 18, 23, 0),
      'W',
      ['7'],
      '08:00',
    );
    expect(dates).toEqual(['20260111', '20260118']);
  });

  it('maps month days beyond the month length and "-1" to the last day', () => {
    const dates = getScheduledDatesWithinRange(
      new Date(2026, 1, 1, 0, 0),
      new Date(2026, 2, 1, 0, 0),
      'M',
      ['31', '-1'],
      '08:00',
    );
    expect(dates).toEqual(['20260228']);
  });

  it('excludes moments before start / after end', () => {
    const dates = getScheduledDatesWithinRange(
      new Date(2026, 0, 10, 12, 0),
      new Date(2026, 0, 12, 1, 0),
      'D',
      [],
      '02:00',
    );
    expect(dates).toEqual(['20260111']);
  });
});

describe('payload builder', () => {
  it('builds the Java ScheduledGraphCreateRequest', () => {
    const payload = buildSchedulePayload(
      base({ cycle: 'M', days: ['15', '-1', '1'], time: '09:30', scheduleDesc: '  nightly ' }),
      { projectId: 'p1', graphId: 'g1', nodes: ['n1', 'n2'] },
    );
    expect(payload).toEqual({
      scheduleId: 'sch-1',
      scheduleDesc: 'nightly',
      cron: {
        startTime: '2026-01-10 12:05:00',
        endTime: '2026-01-20 12:05:00',
        scheduleCycle: 'M',
        scheduleDate: '1,15,end',
        scheduleTime: '09:30:00',
      },
      projectId: 'p1',
      graphId: 'g1',
      nodes: ['n1', 'n2'],
    });
  });

  it('uses an empty scheduleDate for daily and weekday keys for weekly', () => {
    expect(buildScheduleDate('D', ['1'])).toBe('');
    expect(buildScheduleDate('W', ['7', '1', '3'])).toBe('1,3,7');
  });

  it('omits an empty description', () => {
    expect(buildSchedulePayload(base(), { projectId: 'p', graphId: 'g', nodes: [] }).scheduleDesc).toBeUndefined();
  });

  it('parses both datetime-local and space separated values', () => {
    expect(parseDateTime('2026-01-10T12:05')?.getMinutes()).toBe(5);
    expect(parseDateTime('2026-01-10 12:05:30')?.getSeconds()).toBe(30);
    expect(parseDateTime('bad')).toBeNull();
  });

  it('defaults the start to now + 2 minutes', () => {
    expect(initialScheduleValues('x', now).startTime).toBe('2026-01-10T12:02');
  });
});
