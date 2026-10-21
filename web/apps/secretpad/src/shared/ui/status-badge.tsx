import React from "react";
import { Badge } from "@secretpad/design-system";
import { useTranslation } from "../lib/i18n";
const aliases: Record<string, string> = {
  SUCCEED: "SUCCESS",
  SUCCEEDED: "SUCCESS",
  READY: "ACTIVE",
  APPROVED: "SUCCESS",
  AGREE: "SUCCESS",
  INITIALIZED: "RUNNING",
  TERMINATED: "STOPPED",
};
export function StatusBadge({ value }: { value?: string }) {
  const { t } = useTranslation();
  const raw = (value || "UNKNOWN").toUpperCase();
  const status = aliases[raw] || raw;
  const tone = ["SUCCESS", "ACTIVE", "PUBLISHED"].includes(status)
    ? "success"
    : ["FAILED", "ERROR", "REJECTED"].includes(status)
      ? "error"
      : ["RUNNING", "PUBLISHING"].includes(status)
        ? "processing"
        : ["PENDING", "WAITING", "STAGING", "INIT"].includes(status)
          ? "warning"
          : "default";
  const known = [
    "SUCCESS",
    "ACTIVE",
    "PUBLISHED",
    "FAILED",
    "ERROR",
    "REJECTED",
    "RUNNING",
    "PUBLISHING",
    "PENDING",
    "WAITING",
    "STAGING",
    "INIT",
    "STOPPED",
    "ARCHIVED",
    "OFFLINE",
    "UNKNOWN",
    "UNAVAILABLE",
    "DISCARDED",
    "AVAILABLE",
  ];
  return (
    <Badge status={tone}>
      <span title={value}>
        {known.includes(status)
          ? t(`display.status.${status}`)
          : value || t("display.status.UNKNOWN")}
      </span>
    </Badge>
  );
}
