/** Community / help links (legacy header right links). */
export const HELP_LINKS = {
  community: 'https://github.com/c-life',
  help: 'https://www.c-life.com/docs',
};

/** Unread badge text: hidden for 0, capped at `max+`. */
export function formatBadgeCount(count: number, max = 99): string {
  if (!count || count < 0) return '';
  return count > max ? `${max}+` : String(count);
}
