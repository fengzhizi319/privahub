import React from 'react';
import { toast } from '@secretpad/design-system';
import { formatFailedNodes } from '@secretpad/utils';
import { FailedNodesList } from './failed-nodes';

/** Show a warning toast listing failed nodes; returns true if there were failures. */
export function notifyFailedNodes(failed: Record<string, unknown> | null | undefined, title: string): boolean {
  if (formatFailedNodes(failed).length === 0) return false;
  toast.warning(<FailedNodesList failed={failed} title={title} />, 8000);
  return true;
}
