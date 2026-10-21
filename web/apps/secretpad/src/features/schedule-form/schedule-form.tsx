/**
 * Structured periodic-task schedule form (legacy "部署成周期任务" drawer).
 *
 * - `ScheduleFormFields`: controlled fields (id, description, range, cycle, days, time).
 * - `ScheduleCreateDialog`: prefetches `scheduled/id`, checks
 *   `scheduled/graph/once/success`, validates and posts `scheduled/graph/create`.
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  Button,
  CheckboxGroup,
  FormField,
  Input,
  Modal,
  RadioGroup,
  Textarea,
  toast,
} from '@secretpad/design-system';
import {
  createScheduledJava,
  getScheduledIdJava,
  getScheduledOnceSuccessJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import {
  LAST_DAY_OF_MONTH,
  MONTH_DAY_KEYS,
  SCHEDULE_DESC_MAX,
  SCHEDULE_MAX_DAYS,
  WEEK_DAY_KEYS,
  buildSchedulePayload,
  initialScheduleValues,
  isScheduleValid,
  toDateTimeLocal,
  validateSchedule,
  type ScheduleCycle,
  type ScheduleErrors,
  type ScheduleFormValues,
} from './schedule';

const inputCls = 'w-full';

export interface ScheduleFormFieldsProps {
  value: ScheduleFormValues;
  onChange: (value: ScheduleFormValues) => void;
  errors?: ScheduleErrors;
  scheduleIdLoading?: boolean;
}

export const ScheduleFormFields: React.FC<ScheduleFormFieldsProps> = ({
  value,
  onChange,
  errors = {},
  scheduleIdLoading,
}) => {
  const { t } = useTranslation();
  const set = (patch: Partial<ScheduleFormValues>) => onChange({ ...value, ...patch });
  const err = (k: keyof ScheduleErrors) => (errors[k] ? t(errors[k] as string) : undefined);
  const minStart = toDateTimeLocal(new Date());

  const dayOptions = useMemo(() => {
    if (value.cycle === 'W') return WEEK_DAY_KEYS.map((k) => ({ value: k, label: t(`scheduleForm.week.${k}`) }));
    if (value.cycle === 'M')
      return MONTH_DAY_KEYS.map((k) => ({
        value: k,
        label: k === LAST_DAY_OF_MONTH ? t('scheduleForm.lastDay') : k,
      }));
    return [];
  }, [value.cycle, t]);

  return (
    <div className="space-y-4 text-xs">
      <FormField label={t('scheduleForm.scheduleId')} required error={err('scheduleId')}>
        <Input
          className={inputCls}
          value={value.scheduleId}
          disabled
          placeholder={scheduleIdLoading ? t('common.loading') : t('scheduleForm.scheduleIdPlaceholder')}
        />
      </FormField>

      <FormField
        label={t('scheduleForm.desc')}
        error={err('scheduleDesc')}
        help={`${(value.scheduleDesc || '').length}/${SCHEDULE_DESC_MAX}`}
      >
        <Textarea
          className={inputCls}
          rows={2}
          maxLength={SCHEDULE_DESC_MAX}
          value={value.scheduleDesc}
          onChange={(e) => set({ scheduleDesc: e.target.value })}
        />
      </FormField>

      <FormField
        label={t('scheduleForm.range')}
        required
        error={err('startTime') || err('endTime')}
        help={t('scheduleForm.rangeHelp')}
      >
        <div className="flex items-center gap-2">
          <Input
            type="datetime-local"
            aria-label={t('scheduleForm.startTime')}
            min={minStart}
            value={value.startTime}
            onChange={(e) => set({ startTime: e.target.value })}
            invalid={!!errors.startTime}
          />
          <span className="text-gray-400">~</span>
          <Input
            type="datetime-local"
            aria-label={t('scheduleForm.endTime')}
            min={value.startTime || minStart}
            value={value.endTime}
            onChange={(e) => set({ endTime: e.target.value })}
            invalid={!!errors.startTime || !!errors.endTime}
          />
        </div>
      </FormField>

      <FormField label={t('scheduleForm.cycle')} required error={err('cycle')}>
        <RadioGroup
          name={t('scheduleForm.cycle')}
          value={value.cycle}
          onChange={(v) => set({ cycle: v as ScheduleCycle, days: [] })}
          options={[
            { value: 'D', label: t('scheduleForm.cycleD') },
            { value: 'W', label: t('scheduleForm.cycleW') },
            { value: 'M', label: t('scheduleForm.cycleM') },
          ]}
        />
      </FormField>

      {value.cycle && value.cycle !== 'D' && (
        <FormField
          label={t('scheduleForm.days')}
          required
          error={err('days')}
          help={
            value.cycle === 'M'
              ? t('scheduleForm.daysHelpMonth', { max: SCHEDULE_MAX_DAYS })
              : t('scheduleForm.daysHelp', { max: SCHEDULE_MAX_DAYS })
          }
        >
          <CheckboxGroup
            options={dayOptions}
            value={value.days}
            max={SCHEDULE_MAX_DAYS}
            onChange={(days) => set({ days })}
          />
        </FormField>
      )}

      <FormField label={t('scheduleForm.time')} required error={err('time')}>
        <Input
          type="time"
          className="w-40"
          aria-label={t('scheduleForm.time')}
          value={value.time}
          onChange={(e) => set({ time: e.target.value.slice(0, 5) })}
          invalid={!!errors.time}
        />
      </FormField>

      {errors.form && (
        <div className="p-2.5 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900 text-amber-700 dark:text-amber-400">
          {t(errors.form)}
        </div>
      )}
    </div>
  );
};

export interface ScheduleCreateDialogProps {
  isOpen: boolean;
  onClose: () => void;
  projectId: string;
  graphId: string;
  /** graph node ids to schedule (legacy: all graph nodes) */
  nodeIds: string[];
  title?: React.ReactNode;
  /** rendered above the schedule fields (e.g. graph selector / node picker) */
  children?: React.ReactNode;
  onCreated?: () => void;
}

export const ScheduleCreateDialog: React.FC<ScheduleCreateDialogProps> = ({
  isOpen,
  onClose,
  projectId,
  graphId,
  nodeIds,
  title,
  children,
  onCreated,
}) => {
  const { t } = useTranslation();
  const [values, setValues] = useState<ScheduleFormValues>(() => initialScheduleValues());
  const [submitted, setSubmitted] = useState(false);
  const enabled = isOpen && !!projectId && !!graphId;

  const idQuery = useQuery({
    queryKey: ['scheduled-id', projectId, graphId],
    queryFn: () => getScheduledIdJava({ projectId, graphId }),
    enabled,
    staleTime: 0,
    gcTime: 0,
  });

  const onceQuery = useQuery({
    queryKey: ['scheduled-once-success', projectId, graphId],
    queryFn: () => getScheduledOnceSuccessJava({ projectId, graphId }),
    enabled,
  });

  useEffect(() => {
    if (isOpen) {
      setValues(initialScheduleValues());
      setSubmitted(false);
    }
  }, [isOpen, graphId]);

  useEffect(() => {
    if (isOpen) setValues((v) => ({ ...v, scheduleId: idQuery.data || '' }));
  }, [isOpen, idQuery.data]);

  const errors = submitted ? validateSchedule(values) : {};

  const createMutation = useMutation({
    mutationFn: () => createScheduledJava(buildSchedulePayload(values, { projectId, graphId, nodes: nodeIds })),
    onSuccess: () => {
      toast.success(t('periodicTasks.createSuccess'));
      onCreated?.();
      onClose();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const onceSuccess = onceQuery.data === true;
  const onSubmit = () => {
    setSubmitted(true);
    if (!isScheduleValid(validateSchedule(values))) return;
    createMutation.mutate();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={title ?? t('scheduleForm.title')}
      width="max-w-xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            onClick={onSubmit}
            loading={createMutation.isPending}
            disabled={!graphId || !onceSuccess || nodeIds.length === 0 || idQuery.isLoading}
          >
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        {children}
        {graphId && !onceQuery.isLoading && !onceSuccess && (
          <div className="p-2.5 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900 text-amber-700 dark:text-amber-400">
            {t('scheduleForm.onceSuccessHint')}
          </div>
        )}
        {idQuery.error && (
          <div className="text-rose-500">{t('common.error', { message: idQuery.error.message })}</div>
        )}
        {graphId && (
          <ScheduleFormFields
            value={values}
            onChange={setValues}
            errors={errors}
            scheduleIdLoading={idQuery.isLoading}
          />
        )}
      </div>
    </Modal>
  );
};
