import React, {
  Children,
  isValidElement,
  useEffect,
  useRef,
  useState,
} from "react";
import { useTranslation } from "../lib/i18n";
/** Keep the primary action visible; native details remains keyboard operable. */
export function RowActions({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation();
  const root = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    if (!open) return;
    const close = () => {
      const details = root.current?.querySelector("details");
      if (details) details.open = false;
    };
    const click = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) close();
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        const summary = root.current?.querySelector("details[open] summary");
        close();
        (summary as HTMLElement | null)?.focus();
      }
    };
    document.addEventListener("pointerdown", click);
    document.addEventListener("keydown", key);
    window.addEventListener("resize", close);
    return () => {
      document.removeEventListener("pointerdown", click);
      document.removeEventListener("keydown", key);
      window.removeEventListener("resize", close);
    };
  }, [open]);
  const flatten = (nodes: React.ReactNode): React.ReactNode[] =>
    Children.toArray(nodes).flatMap((node) =>
      isValidElement<{ children?: React.ReactNode }>(node) &&
      node.type === React.Fragment
        ? flatten(node.props.children)
        : [node],
    );
  const actions = flatten(children);
  return (
    <div ref={root} className="flex items-center gap-2 whitespace-nowrap">
      {actions[0]}
      {actions.length > 1 && (
        <details
          className="relative"
          onToggle={(event) => {
            setOpen(event.currentTarget.open);
            if (event.currentTarget.open) {
              const rect = event.currentTarget.getBoundingClientRect();
              const menu =
                event.currentTarget.querySelector<HTMLElement>(
                  "[data-action-menu]",
                );
              if (menu) {
                menu.style.top = `${Math.min(rect.bottom + 4, window.innerHeight - 220)}px`;
                menu.style.left = `${Math.max(8, rect.right - 170)}px`;
              }
            }
          }}
        >
          <summary className="cursor-pointer rounded-lg px-2 py-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800">
            {t("display.more")}
          </summary>
          <div
            data-action-menu
            className="fixed z-40 flex w-40 flex-col items-stretch gap-1 rounded-xl border border-gray-200 bg-white p-2 shadow-lg dark:border-gray-700 dark:bg-gray-900"
            onClick={(event) => {
              const details = event.currentTarget.closest("details");
              if (details) details.open = false;
            }}
          >
            {actions.slice(1)}
          </div>
        </details>
      )}
    </div>
  );
}
