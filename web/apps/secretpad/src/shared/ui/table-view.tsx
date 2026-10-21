import React, {
  Children,
  cloneElement,
  isValidElement,
  useEffect,
  useState,
} from "react";
import { useTranslation } from "../lib/i18n";

type Element = React.ReactElement<Record<string, unknown>>;
const elements = (node: React.ReactNode) =>
  Children.toArray(node).filter(isValidElement) as Element[];
function cellText(node: React.ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(cellText).join(" ");
  return isValidElement<{ children?: React.ReactNode }>(node)
    ? cellText(node.props.children)
    : "";
}
interface Preferences {
  density: "compact" | "comfortable";
  hidden: number[];
  widths: Record<string, number>;
}
const defaults: Preferences = { density: "compact", hidden: [], widths: {} };
function readPreferences(id: string): Preferences {
  try {
    const value = JSON.parse(
      localStorage.getItem(`privahub-table-${id}`) || "null",
    );
    if (!value || !Array.isArray(value.hidden) || !value.widths)
      return defaults;
    return {
      density: value.density === "comfortable" ? "comfortable" : "compact",
      hidden: value.hidden.filter((n: unknown) => Number.isInteger(n)),
      widths: Object.fromEntries(
        Object.entries(value.widths).filter(
          ([, n]) => typeof n === "number" && n >= 80 && n <= 800,
        ),
      ) as Record<string, number>,
    };
  } catch {
    return defaults;
  }
}

/** Enhances native tables without changing their API paging or row actions.
 * Sorting applies to the loaded page and is labelled explicitly; existing server
 * sort handlers are preserved. Only display preferences are stored locally.
 */
export function TableView({
  id,
  children,
}: {
  id: string;
  children: React.ReactElement;
}) {
  const { t } = useTranslation();
  const [prefs, setPrefs] = useState(() => readPreferences(id));
  const [sort, setSort] = useState<{
    column: number;
    direction: number;
  } | null>(null);
  useEffect(() => {
    try {
      localStorage.setItem(`privahub-table-${id}`, JSON.stringify(prefs));
    } catch {
      /* Optional browser storage. */
    }
  }, [id, prefs]);
  const table = children as Element;
  const sections = elements(table.props.children as React.ReactNode);
  const head = sections.find((section) => section.type === "thead");
  const headerRow = head && elements(head.props.children as React.ReactNode)[0];
  const headers = headerRow
    ? elements(headerRow.props.children as React.ReactNode)
    : [];
  const hidden = prefs.hidden.filter(
    (index) => index > 0 && index < headers.length - 1,
  );
  const change = (patch: Partial<Preferences>) =>
    setPrefs((previous) => ({ ...previous, ...patch }));
  const transformRow = (row: Element, header = false) =>
    cloneElement(
      row,
      {},
      elements(row.props.children as React.ReactNode).flatMap((cell, index) => {
        if (Number(cell.props.colSpan) > 1)
          return [
            cloneElement(cell, {
              colSpan: Math.max(1, headers.length - hidden.length),
            }),
          ];
        if (hidden.includes(index)) return [];
        const label = cellText(cell.props.children as React.ReactNode);
        const width = prefs.widths[index];
        const props: Record<string, unknown> = {
          style: {
            ...(cell.props.style as object),
            ...(width ? { width, minWidth: width, maxWidth: width } : {}),
          },
        };
        if (
          header &&
          label &&
          index < headers.length - 1 &&
          !cell.props.onClick
        ) {
          props["aria-sort"] =
            sort?.column === index
              ? sort.direction === 1
                ? "ascending"
                : "descending"
              : "none";
          return [
            cloneElement(
              cell,
              props,
              <button
                type="button"
                className="inline-flex items-center gap-2 text-left"
                title={t("display.pageSort")}
                onClick={() =>
                  setSort((previous) =>
                    previous?.column === index
                      ? previous.direction === 1
                        ? { column: index, direction: -1 }
                        : null
                      : { column: index, direction: 1 },
                  )
                }
              >
                {cell.props.children as React.ReactNode}
                <span aria-hidden="true" className="text-gray-400">
                  {sort?.column === index
                    ? sort.direction === 1
                      ? "↑"
                      : "↓"
                    : "↕"}
                </span>
              </button>,
            ),
          ];
        }
        return [cloneElement(cell, props)];
      }),
    );
  const enhanced = sections.map((section) => {
    if (section.type === "thead")
      return cloneElement(
        section,
        {},
        headerRow && transformRow(headerRow, true),
      );
    if (section.type !== "tbody") return section;
    let rows = elements(section.props.children as React.ReactNode);
    if (
      sort &&
      rows.every(
        (row) =>
          elements(row.props.children as React.ReactNode).length ===
          headers.length,
      )
    ) {
      const collator = new Intl.Collator(undefined, {
        numeric: true,
        sensitivity: "base",
      });
      rows = [...rows].sort(
        (a, b) =>
          collator.compare(
            cellText(
              elements(a.props.children as React.ReactNode)[sort.column]?.props
                .children as React.ReactNode,
            ),
            cellText(
              elements(b.props.children as React.ReactNode)[sort.column]?.props
                .children as React.ReactNode,
            ),
          ) * sort.direction,
      );
    }
    return cloneElement(
      section,
      {},
      rows.map((row) => transformRow(row)),
    );
  });
  return (
    <div className="table-view" data-density={prefs.density}>
      <div className="flex flex-wrap items-center justify-end gap-3 border-b border-gray-100 px-3 py-2 text-xs text-gray-500 dark:border-gray-800">
        <span>{t("display.pageSort")}</span>
        <label>
          {t("display.density")}{" "}
          <select
            aria-label={t("display.density")}
            value={prefs.density}
            onChange={(event) =>
              change({ density: event.target.value as Preferences["density"] })
            }
            className="rounded border border-gray-200 bg-transparent px-2 py-1 dark:border-gray-700"
          >
            <option value="compact">{t("display.compact")}</option>
            <option value="comfortable">{t("display.comfortable")}</option>
          </select>
        </label>
        <details className="relative">
          <summary className="cursor-pointer rounded border border-gray-200 px-2 py-1 dark:border-gray-700">
            {t("display.columns")}
          </summary>
          <div className="absolute right-0 z-30 mt-2 w-72 space-y-3 rounded-xl border border-gray-200 bg-white p-4 shadow-xl dark:border-gray-700 dark:bg-gray-900">
            {headers.map((header, index) => {
              const label = cellText(header.props.children as React.ReactNode);
              return (
                label && (
                  <div key={index} className="space-y-1">
                    <label className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={!hidden.includes(index)}
                        disabled={index === 0 || index === headers.length - 1}
                        onChange={(event) =>
                          change({
                            hidden: event.target.checked
                              ? hidden.filter((value) => value !== index)
                              : [...hidden, index],
                          })
                        }
                      />
                      {label}
                    </label>
                    <input
                      type="range"
                      aria-label={`${label} · ${t("display.width")}`}
                      min={80}
                      max={800}
                      step={20}
                      value={prefs.widths[index] || 160}
                      onChange={(event) =>
                        change({
                          widths: {
                            ...prefs.widths,
                            [index]: Number(event.target.value),
                          },
                        })
                      }
                      className="w-full"
                    />
                  </div>
                )
              );
            })}
            <button
              type="button"
              onClick={() => {
                setPrefs(defaults);
                setSort(null);
              }}
              className="text-blue-600"
            >
              {t("display.reset")}
            </button>
          </div>
        </details>
      </div>
      <div className="max-h-[70vh] overflow-auto">
        {cloneElement(table, {}, enhanced)}
      </div>
    </div>
  );
}
