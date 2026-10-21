import React from 'react';
import { formatFailedNodes } from '@secretpad/utils';

/** Per-node failure list for Java `failedCreatedNodes` (partial success). Hook-free so it can render inside toasts. */
export const FailedNodesList: React.FC<{ failed?: Record<string, unknown> | null; title: React.ReactNode }> = ({
  failed,
  title,
}) => {
  const items = formatFailedNodes(failed);
  if (items.length === 0) return null;
  return (
    <div className="space-y-1 text-xs" role="alert">
      <div className="font-semibold">{title}</div>
      <ul className="space-y-0.5">
        {items.map((n) => (
          <li key={n.nodeId} className="break-all">
            <span className="font-mono font-semibold">{n.nodeId}</span>
            {n.message ? `: ${n.message}` : ''}
          </li>
        ))}
      </ul>
    </div>
  );
};
