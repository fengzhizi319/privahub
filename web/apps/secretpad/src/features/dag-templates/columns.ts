/** Both participants contribute features to a vertical model. Preserve table
 * order while deduplicating shared column names (for example aligned IDs).
 */
export function mergeTableColumns<T extends { name: string }>(...tables: (T[] | undefined)[]): T[] {
  return [...new Map(tables.flatMap(columns => columns ?? []).map(column => [column.name, column])).values()];
}
