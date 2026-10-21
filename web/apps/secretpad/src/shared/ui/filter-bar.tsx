import React from "react";
import { useTranslation } from "../lib/i18n";
/** Consistent wrapping and reset affordance; project/node scope stays selected. */
export function FilterBar({
  children,
  onReset,
}: {
  children: React.ReactNode;
  onReset: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      role="group"
      aria-label={t("display.filters")}
      className="flex flex-wrap items-center gap-3 rounded-xl border border-gray-100 bg-gray-50/60 p-3 dark:border-gray-800 dark:bg-gray-900/50"
    >
      {children}
      <button
        type="button"
        onClick={onReset}
        className="ml-auto shrink-0 rounded-lg px-3 py-2 text-xs font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-300 dark:hover:bg-blue-950"
      >
        {t("display.clearFilters")}
      </button>
    </div>
  );
}
