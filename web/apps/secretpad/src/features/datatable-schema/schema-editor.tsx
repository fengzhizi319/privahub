/**
 * 数据表结构编辑器（旧前端 `DataTableStructure` / `UploadTable` 字段配置的迁移版）。
 *
 * - 每行：特征名称、类型（integer/float/string）、描述；
 * - 校验：必填、64 字符、英文开头仅含字母数字 `_` `-`、不可重复、描述 200 字符；
 * - SCQL 提示：命中 SQL 关键字或含中划线时以告警样式标注（不阻止提交）；
 * - 支持“仅看错误”、样例文件下载与上传数据表结构 CSV（特征名称,特征类型,特征描述）。
 */
import React, { useMemo, useRef, useState } from 'react';
import { Button, Input, Select, toast } from '@secretpad/design-system';
import { SCHEMA_TYPE_OPTIONS, parseSchemaCsv, scqlWarningIndexes, validateSchemaFields } from '@secretpad/utils';
import { useTranslation } from '@/shared/lib/i18n';
import { downloadSchemaTemplate, emptySchemaField, readCsvHead } from './model';
import type { SchemaField } from './model';

export interface SchemaEditorProps {
  value: SchemaField[];
  onChange: (fields: SchemaField[]) => void;
  /** Show validation errors (after the first submit attempt / blur). */
  showErrors?: boolean;
  /** Allow adding / removing rows (registration). Upload keeps the CSV header rows. */
  editableRows?: boolean;
}

export const SchemaEditor: React.FC<SchemaEditorProps> = ({ value, onChange, showErrors = true, editableRows = true }) => {
  const { t } = useTranslation();
  const [onlyErrors, setOnlyErrors] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const issues = useMemo(() => validateSchemaFields(value), [value]);
  const warnings = useMemo(() => new Set(scqlWarningIndexes(value)), [value]);
  const issuesByRow = useMemo(() => {
    const m = new Map<number, string[]>();
    issues.forEach((i) => m.set(i.index, [...(m.get(i.index) || []), t(`schemaEditor.err.${i.kind}`)]));
    return m;
  }, [issues, t]);
  const errorRowCount = issuesByRow.size;

  const update = (idx: number, patch: Partial<SchemaField>) =>
    onChange(value.map((f, i) => (i === idx ? { ...f, ...patch } : f)));

  const handleSchemaCsv = async (file?: File | null) => {
    if (!file) return;
    try {
      const preview = await readCsvHead(file, 4 * 1024 * 1024, 100000);
      const parsed = parseSchemaCsv(preview);
      if (!parsed) {
        toast.error(t('schemaEditor.csvFormatError'));
        return;
      }
      onChange(parsed.fields.length > 0 ? parsed.fields : [emptySchemaField()]);
      toast.success(
        parsed.duplicates > 0
          ? t('schemaEditor.csvUploadedDedup', { n: parsed.fields.length + parsed.duplicates, d: parsed.duplicates })
          : t('schemaEditor.csvUploaded', { n: parsed.fields.length }),
      );
    } catch {
      toast.error(t('schemaEditor.csvFormatError'));
    } finally {
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const typeOptions = SCHEMA_TYPE_OPTIONS.map((o) => ({ value: o.value, label: o.label }));

  return (
    <div className="space-y-2 text-xs">
      <div className="flex items-center justify-between">
        <span className="font-semibold text-gray-800 dark:text-gray-200">{t('schemaEditor.title')}</span>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="link" onClick={() => downloadSchemaTemplate()}>
            {t('schemaEditor.downloadTemplate')}
          </Button>
          <Button size="sm" variant="link" onClick={() => fileRef.current?.click()}>
            {t('schemaEditor.uploadSchema')}
          </Button>
          <input
            ref={fileRef}
            type="file"
            accept=".csv"
            className="hidden"
            data-testid="schema-csv-input"
            onChange={(e) => handleSchemaCsv(e.target.files?.[0])}
          />
        </div>
      </div>

      <div className="px-3 py-2 rounded-lg bg-amber-50 dark:bg-amber-950/30 text-amber-700 dark:text-amber-300">
        {t('schemaEditor.scqlTip')}
      </div>

      {showErrors && errorRowCount > 0 && (
        <div className="flex items-center justify-between px-3 py-2 rounded-lg bg-rose-50 dark:bg-rose-950/30 text-rose-600 dark:text-rose-300" role="alert">
          <span>{t('schemaEditor.errorSummary', { total: value.length, n: errorRowCount })}</span>
          <label className="inline-flex items-center gap-1 cursor-pointer">
            <input type="checkbox" checked={onlyErrors} onChange={(e) => setOnlyErrors(e.target.checked)} />
            {t('schemaEditor.onlyErrors')}
          </label>
        </div>
      )}
      {showErrors && value.length === 0 && <div className="text-rose-500">{t('schemaEditor.err.empty')}</div>}

      <div className="grid grid-cols-[minmax(0,2fr)_110px_minmax(0,3fr)_32px] gap-2 px-2 py-1.5 bg-gray-50 dark:bg-gray-800 rounded font-semibold text-gray-500">
        <span>{t('schemaEditor.featureName')}</span>
        <span>{t('schemaEditor.featureType')}</span>
        <span>{t('schemaEditor.featureDesc')}</span>
        <span />
      </div>

      {editableRows && (
        <Button size="sm" variant="outline" className="w-full border-dashed" onClick={() => onChange([...value, emptySchemaField()])}>
          ＋ {t('schemaEditor.add')}
        </Button>
      )}

      <div className="space-y-1.5 max-h-[40vh] overflow-y-auto pr-1">
        {value.map((f, idx) => {
          const rowErrors = showErrors ? issuesByRow.get(idx) : undefined;
          if (onlyErrors && showErrors && !rowErrors) return null;
          const warn = warnings.has(idx);
          return (
            <div key={idx}>
              <div className="grid grid-cols-[minmax(0,2fr)_110px_minmax(0,3fr)_32px] gap-2 items-center px-2">
                <Input
                  aria-label={`${t('schemaEditor.featureName')} ${idx + 1}`}
                  value={f.featureName}
                  invalid={!!rowErrors?.length}
                  className={warn && !rowErrors?.length ? '!border-amber-400' : ''}
                  placeholder={t('dsForm.placeholder')}
                  onChange={(e) => update(idx, { featureName: e.target.value })}
                />
                <Select
                  aria-label={`${t('schemaEditor.featureType')} ${idx + 1}`}
                  value={f.featureType}
                  placeholder={t('dsForm.selectPlaceholder')}
                  options={typeOptions}
                  onChange={(v) => update(idx, { featureType: v })}
                />
                <Input
                  aria-label={`${t('schemaEditor.featureDesc')} ${idx + 1}`}
                  value={f.featureDescription}
                  maxLength={200}
                  placeholder={t('dsForm.placeholder')}
                  onChange={(e) => update(idx, { featureDescription: e.target.value })}
                />
                {editableRows ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={t('common.delete')}
                    onClick={() => onChange(value.filter((_, i) => i !== idx))}
                  >
                    ✕
                  </Button>
                ) : (
                  <span />
                )}
              </div>
              {rowErrors && rowErrors.length > 0 && (
                <div className="px-2 pt-0.5 text-[11px] text-rose-500">{rowErrors.join('；')}</div>
              )}
              {!rowErrors?.length && warn && (
                <div className="px-2 pt-0.5 text-[11px] text-amber-600">{t('schemaEditor.scqlWarn')}</div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
};

/** 空缺值（nullStrs）编辑框：逗号分隔的 JSON 字符串，默认 `""`。 */
export const NullStrsField: React.FC<{
  value: string;
  onChange: (v: string) => void;
  error?: boolean;
}> = ({ value, onChange, error }) => {
  const { t } = useTranslation();
  return (
    <div className="space-y-1 text-xs">
      <label className="block font-medium text-gray-700 dark:text-gray-300">{t('schemaEditor.nullStrs')}</label>
      <textarea
        aria-label={t('schemaEditor.nullStrs')}
        value={value}
        rows={2}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t('schemaEditor.nullStrsPlaceholder')}
        className={`w-full p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border font-mono focus:outline-none focus:border-blue-500 ${
          error ? 'border-rose-400' : 'border-gray-200 dark:border-gray-700'
        }`}
      />
      <div className={error ? 'text-rose-500' : 'text-gray-400'}>
        {error ? t('schemaEditor.nullStrsInvalid') : t('schemaEditor.nullStrsHelp')}
      </div>
    </div>
  );
};
