import { useEffect, useRef } from 'react';

// Shared stack keeps an Escape / Tab event inside the topmost dialog when a
// drawer opens a nested confirmation modal. Each instance owns its listeners.
const activeDialogs: HTMLElement[] = [];
const selector = 'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** Manage keyboard focus for controlled modal surfaces without closing their
 * parent. Call before the conditional render; focus returns to the opener on
 * close. The callback ref avoids restoring focus on every parent rerender. */
export function useDialogFocus(open: boolean, onClose: () => void) {
  const surface = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  useEffect(() => { close.current = onClose; }, [onClose]);
  useEffect(() => {
    const element = surface.current;
    if (!open || !element) return;
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    activeDialogs.push(element);
    const focusable = () => Array.from(element.querySelectorAll<HTMLElement>(selector))
      .filter(node => node.getClientRects().length > 0 && node.tabIndex >= 0);
    const focusFirst = () => (focusable()[0] || element).focus();
    focusFirst();
    const topmost = () => activeDialogs[activeDialogs.length - 1] === element;
    const keydown = (event: KeyboardEvent) => {
      if (!topmost()) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopImmediatePropagation();
        close.current();
      } else if (event.key === 'Tab') {
        const nodes = focusable();
        const first = nodes[0], last = nodes[nodes.length - 1];
        if (!first) { event.preventDefault(); element.focus(); }
        else if (event.shiftKey && (document.activeElement === first || document.activeElement === element)) {
          event.preventDefault(); last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault(); first.focus();
        }
      }
    };
    const focusin = (event: FocusEvent) => {
      if (topmost() && event.target instanceof Node && !element.contains(event.target)) focusFirst();
    };
    document.addEventListener('keydown', keydown, true);
    document.addEventListener('focusin', focusin);
    return () => {
      document.removeEventListener('keydown', keydown, true);
      document.removeEventListener('focusin', focusin);
      const index = activeDialogs.indexOf(element);
      if (index !== -1) activeDialogs.splice(index, 1);
      if (previous?.isConnected) previous.focus();
    };
  }, [open]);
  return surface;
}
