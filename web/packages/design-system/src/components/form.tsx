import React from 'react';
import { useDialogFocus } from './use-dialog-focus';

/**
 * Form & layout primitives used by the migrated business pages
 * (wizards, filters, drawers). Tailwind based, dark-mode aware, controlled.
 */

const fieldBase =
  'w-full px-3 py-1.5 text-sm rounded-lg bg-surface-input border text-fg-primary placeholder:text-gray-400 focus:outline-none focus:border-blue-500 disabled:opacity-60 disabled:cursor-not-allowed transition-colors';

function borderClass(invalid?: boolean) {
  return invalid ? 'border-rose-400 dark:border-rose-600' : 'border-gray-200 dark:border-gray-700';
}

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean;
}

export const Input = React.forwardRef<HTMLInputElement, InputProps>(({ invalid, className = '', ...props }, ref) => (
  <input ref={ref} className={`${fieldBase} ${borderClass(invalid)} ${className}`} {...props} />
));
Input.displayName = 'Input';

export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  invalid?: boolean;
}

export const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ invalid, className = '', ...props }, ref) => (
    <textarea ref={ref} className={`${fieldBase} ${borderClass(invalid)} ${className}`} {...props} />
  ),
);
Textarea.displayName = 'Textarea';

export interface SelectOption {
  value: string;
  label: React.ReactNode;
  disabled?: boolean;
}

export interface SelectProps extends Omit<React.SelectHTMLAttributes<HTMLSelectElement>, 'onChange'> {
  options: SelectOption[];
  placeholder?: string;
  invalid?: boolean;
  onChange?: (value: string) => void;
}

export const Select: React.FC<SelectProps> = ({ options, placeholder, invalid, className = '', onChange, ...props }) => (
  <select
    className={`${fieldBase} ${borderClass(invalid)} ${className}`}
    onChange={(e) => onChange?.(e.target.value)}
    {...props}
  >
    {placeholder !== undefined && <option value="">{placeholder}</option>}
    {options.map((o) => (
      <option key={o.value} value={o.value} disabled={o.disabled}>
        {typeof o.label === 'string' || typeof o.label === 'number' ? o.label : o.value}
      </option>
    ))}
  </select>
);

/** Checkbox list for multi-select (e.g. participant nodes). */
export interface CheckboxGroupProps {
  options: SelectOption[];
  value: string[];
  onChange: (value: string[]) => void;
  max?: number;
  className?: string;
}

export const CheckboxGroup: React.FC<CheckboxGroupProps> = ({ options, value, onChange, max, className = '' }) => (
  <div className={`flex flex-wrap gap-2 ${className}`}>
    {options.map((o) => {
      const checked = value.includes(o.value);
      const disabled = o.disabled || (!checked && max !== undefined && value.length >= max);
      return (
        <label
          key={o.value}
          className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg border text-xs cursor-pointer select-none ${
            checked
              ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300'
              : 'border-gray-200 dark:border-gray-700 text-gray-700 dark:text-gray-300'
          } ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
        >
          <input
            type="checkbox"
            className="accent-blue-600"
            checked={checked}
            disabled={disabled}
            onChange={(e) => onChange(e.target.checked ? [...value, o.value] : value.filter((v) => v !== o.value))}
          />
          {o.label}
        </label>
      );
    })}
  </div>
);

/** Segmented radio control. */
export interface RadioGroupProps {
  options: SelectOption[];
  value: string;
  onChange: (value: string) => void;
  className?: string;
  name?: string;
}

export const RadioGroup: React.FC<RadioGroupProps> = ({ options, value, onChange, className = '', name }) => (
  <div role="radiogroup" aria-label={name} className={`inline-flex flex-wrap gap-1.5 ${className}`}>
    {options.map((o) => (
      <button
        type="button"
        role="radio"
        aria-checked={value === o.value}
        key={o.value}
        disabled={o.disabled}
        onClick={() => onChange(o.value)}
        className={`px-3 py-1 text-xs rounded-lg border transition-colors disabled:opacity-50 ${
          value === o.value
            ? 'border-blue-500 bg-blue-600 text-white'
            : 'border-gray-200 dark:border-gray-700 text-gray-700 dark:text-gray-300 hover:border-blue-400'
        }`}
      >
        {o.label}
      </button>
    ))}
  </div>
);

/** Label + control + help/error line. */
export interface FormFieldProps {
  label?: React.ReactNode;
  required?: boolean;
  error?: React.ReactNode;
  help?: React.ReactNode;
  htmlFor?: string;
  children: React.ReactNode;
  className?: string;
}

export const FormField: React.FC<FormFieldProps> = ({ label, required, error, help, htmlFor, children, className = '' }) => (
  <div className={`space-y-1 ${className}`}>
    {label && (
      <label htmlFor={htmlFor} className="block text-xs font-medium text-gray-700 dark:text-gray-300">
        {required && <span className="text-rose-500 mr-0.5">*</span>}
        {label}
      </label>
    )}
    {children}
    {error ? (
      <div className="text-[11px] text-rose-500" role="alert">
        {error}
      </div>
    ) : help ? (
      <div className="text-[11px] text-gray-400">{help}</div>
    ) : null}
  </div>
);

/** Controlled tabs header (content rendered by caller). */
export interface TabItem {
  key: string;
  label: React.ReactNode;
  badge?: number;
}

export interface TabsProps {
  items: TabItem[];
  activeKey: string;
  onChange: (key: string) => void;
  className?: string;
}

export const Tabs: React.FC<TabsProps> = ({ items, activeKey, onChange, className = '' }) => (
  <div role="tablist" className={`flex gap-1 overflow-x-auto border-b border-gray-200 dark:border-gray-800 ${className}`}>
    {items.map((it) => (
      <button
        key={it.key}
        type="button"
        role="tab"
        aria-selected={activeKey === it.key}
        onClick={() => onChange(it.key)}
        className={`shrink-0 whitespace-nowrap px-3 py-2 text-sm -mb-px border-b-2 transition-colors ${
          activeKey === it.key
            ? 'border-blue-600 text-blue-600 dark:text-blue-400 font-medium'
            : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
        }`}
      >
        {it.label}
        {it.badge ? (
          <span className="ml-1.5 px-1.5 rounded-full bg-rose-500 text-white text-[10px]">{it.badge}</span>
        ) : null}
      </button>
    ))}
  </div>
);

/** Right-side drawer (controlled). */
export interface DrawerProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  width?: string;
}

export const Drawer: React.FC<DrawerProps> = ({ isOpen, onClose, title, children, footer, width = 'max-w-xl' }) => {
  const dialogId = React.useId();
  const dialogRef = useDialogFocus(isOpen, onClose);
  if (!isOpen) return null;
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40" onClick={onClose}>
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? dialogId : undefined}
        aria-label={title ? undefined : 'Dialog'}
        tabIndex={-1}
        className={`w-full ${width} h-full bg-white dark:bg-gray-900 border-l border-gray-200 dark:border-gray-800 shadow-xl flex flex-col`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-4 sm:px-5 py-3.5 gap-2 border-b border-gray-100 dark:border-gray-800 flex items-center justify-between">
          <h3 id={dialogId} className="min-w-0 truncate font-semibold text-base text-gray-900 dark:text-gray-100">{title}</h3>
          <button type="button" onClick={onClose} aria-label="close" className="shrink-0 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200">
            ✕
          </button>
        </div>
        <div className="flex-1 overflow-y-auto p-4 sm:p-5">{children}</div>
        {footer && (
          <div className="px-4 sm:px-5 py-3 border-t border-gray-100 dark:border-gray-800 flex flex-wrap justify-end gap-2">{footer}</div>
        )}
      </div>
    </div>
  );
};

/** Simple pager: « prev | page / total | next ». */
export interface PaginationProps {
  page: number;
  pageSize: number;
  total: number;
  onChange: (page: number) => void;
  className?: string;
}

export const Pagination: React.FC<PaginationProps> = ({ page, pageSize, total, onChange, className = '' }) => {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)));
  return (
    <div className={`flex items-center justify-end gap-2 text-xs text-gray-500 ${className}`}>
      <span>
        {total} · {page}/{pages}
      </span>
      <button
        type="button"
        className="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 disabled:opacity-40"
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
        aria-label="previous page"
      >
        ‹
      </button>
      <button
        type="button"
        className="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 disabled:opacity-40"
        disabled={page >= pages}
        onClick={() => onChange(page + 1)}
        aria-label="next page"
      >
        ›
      </button>
    </div>
  );
};

/** Horizontal step indicator for wizards. */
export interface StepsProps {
  steps: React.ReactNode[];
  current: number;
  className?: string;
}

export const Steps: React.FC<StepsProps> = ({ steps, current, className = '' }) => (
  <ol className={`flex items-center gap-2 text-xs ${className}`}>
    {steps.map((s, i) => (
      <li key={i} className="flex items-center gap-2">
        <span
          className={`w-5 h-5 rounded-full flex items-center justify-center font-semibold ${
            i < current
              ? 'bg-emerald-500 text-white'
              : i === current
                ? 'bg-blue-600 text-white'
                : 'bg-gray-200 dark:bg-gray-700 text-gray-500'
          }`}
        >
          {i < current ? '✓' : i + 1}
        </span>
        <span className={i === current ? 'text-gray-900 dark:text-gray-100 font-medium' : 'text-gray-500'}>{s}</span>
        {i < steps.length - 1 && <span className="w-6 h-px bg-gray-300 dark:bg-gray-700" />}
      </li>
    ))}
  </ol>
);

/** Empty placeholder. */
export const Empty: React.FC<{ children?: React.ReactNode; className?: string }> = ({ children, className = '' }) => (
  <div className={`py-10 text-center text-sm text-gray-400 ${className}`}>{children ?? '—'}</div>
);

/** Copy-to-clipboard inline button. */
export const CopyButton: React.FC<{ text: string; label?: React.ReactNode; onCopied?: () => void; className?: string }> = ({
  text,
  label = '⧉',
  onCopied,
  className = '',
}) => (
  <button
    type="button"
    className={`text-xs text-blue-600 hover:underline ${className}`}
    onClick={() => {
      void navigator.clipboard?.writeText(text).then(() => onCopied?.());
    }}
  >
    {label}
  </button>
);
