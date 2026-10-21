import React from "react";
import { useTranslation } from "../lib/i18n";
/** Distinguish waiting, failure and an actual empty result set. Never show a
 * success-style empty state for a failed request. Retry repeats the same scope.
 */
export function QueryFeedback({
  loading,
  error,
  empty,
  label,
  onRetry,
}: {
  loading?: boolean;
  error?: unknown;
  empty?: boolean;
  label?: string;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  if (error)
    return (
      <div
        role="alert"
        className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700 dark:border-rose-900 dark:bg-rose-950/30 dark:text-rose-300"
      >
        <p>
          {t("common.error", {
            message: error instanceof Error ? error.message : String(error),
          })}
        </p>
        {onRetry && (
          <button
            type="button"
            onClick={onRetry}
            className="mt-2 font-semibold underline"
          >
            {t("common.refresh")}
          </button>
        )}
      </div>
    );
  if (loading)
    return (
      <div role="status" className="space-y-2 py-4">
        <span className="text-xs text-gray-500">{t("common.loading")}</span>
        <div
          aria-hidden="true"
          className="mx-auto h-2 max-w-xs animate-pulse rounded bg-gray-200 dark:bg-gray-700"
        />
      </div>
    );
  if (empty)
    return (
      <div className="py-8 text-center text-sm text-gray-500">
        <span aria-hidden="true" className="mb-2 block text-2xl text-gray-300">
          ◇
        </span>
        {label || t("common.empty")}
      </div>
    );
  return null;
}
