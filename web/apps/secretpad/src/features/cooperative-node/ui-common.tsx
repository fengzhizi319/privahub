import React, { useState } from 'react';
import { Badge, CopyButton, Input, Select, toast } from '@secretpad/design-system';
import { useTranslation } from '@/shared/lib/i18n';
import type { NetProtocol } from './auth-code';
import { nodeStatusMeta } from './format';

export const NodeStatusBadge: React.FC<{ status?: string }> = ({ status }) => {
  const { t } = useTranslation();
  const meta = nodeStatusMeta(status);
  return (
    <Badge status={meta.badge}>
      <span title={status}>{t(meta.key)}</span>
    </Badge>
  );
};

/**
 * Secret / long text (cert, public key, auth code, token) shown collapsed
 * with "view" + "copy" actions (legacy PopoverCopy).
 */
export const SecretText: React.FC<{ text?: string; emptyHint?: React.ReactNode; className?: string }> = ({
  text,
  emptyHint,
  className = '',
}) => {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  if (!text) return <span className="text-gray-400">{emptyHint ?? '-'}</span>;
  return (
    <div className={`min-w-0 ${className}`}>
      <div className="flex items-center gap-3">
        <button type="button" className="text-xs text-blue-600 hover:underline" onClick={() => setOpen((v) => !v)}>
          {open ? t('coop.hide') : t('coop.view')}
        </button>
        <CopyButton text={text} label={t('coop.copy')} onCopied={() => toast.success(t('coop.copied'))} />
      </div>
      {open && (
        <pre className="mt-1.5 p-2 rounded bg-gray-50 dark:bg-gray-800 border border-gray-100 dark:border-gray-700 text-[10px] font-mono whitespace-pre-wrap break-all max-h-48 overflow-auto">
          {text}
        </pre>
      )}
    </div>
  );
};

/** host:port input with an http(s):// prefix select (legacy SelectBefore). */
export const AddressInput: React.FC<{
  protocol: NetProtocol;
  onProtocolChange: (p: NetProtocol) => void;
  value: string;
  onChange: (v: string) => void;
  invalid?: boolean;
  placeholder?: string;
  disabled?: boolean;
}> = ({ protocol, onProtocolChange, value, onChange, invalid, placeholder, disabled }) => (
  <div className="flex gap-1.5">
    <Select
      className="!w-28 shrink-0"
      value={protocol}
      disabled={disabled}
      onChange={(v) => onProtocolChange(v as NetProtocol)}
      options={[
        { value: 'http://', label: 'http://' },
        { value: 'https://', label: 'https://' },
      ]}
    />
    <Input
      value={value}
      invalid={invalid}
      disabled={disabled}
      placeholder={placeholder ?? '127.0.0.1:1080'}
      onChange={(e) => onChange(e.target.value.trim())}
    />
  </div>
);

/** Label/value row for detail drawers. */
export const InfoRow: React.FC<{ label: React.ReactNode; children: React.ReactNode }> = ({ label, children }) => (
  <div className="grid grid-cols-[8rem_1fr] gap-3 py-1.5 text-xs">
    <div className="text-gray-500">{label}</div>
    <div className="text-gray-800 dark:text-gray-200 min-w-0 break-all">{children}</div>
  </div>
);
