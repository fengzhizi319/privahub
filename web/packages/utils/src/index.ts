/**
 * SecretPad Shared Utilities Package
 */

export * from './crypto';

// Formatters
export function formatDate(date: Date | string | number, format = 'YYYY-MM-DD HH:mm:ss'): string {
  const d = new Date(date);
  if (isNaN(d.getTime())) return '-';
  const pad = (n: number) => n.toString().padStart(2, '0');
  
  return format
    .replace('YYYY', d.getFullYear().toString())
    .replace('MM', pad(d.getMonth() + 1))
    .replace('DD', pad(d.getDate()))
    .replace('HH', pad(d.getHours()))
    .replace('mm', pad(d.getMinutes()))
    .replace('ss', pad(d.getSeconds()));
}

export function formatBytes(bytes: number, decimals = 2): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

// Storage wrapper
export const storage = {
  get<T>(key: string, defaultValue: T): T {
    try {
      const item = localStorage.getItem(key);
      return item ? JSON.parse(item) : defaultValue;
    } catch {
      return defaultValue;
    }
  },
  set<T>(key: string, value: T): void {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch (e) {
      console.warn(`[storage] Failed to set ${key}`, e);
    }
  },
  remove(key: string): void {
    try {
      localStorage.removeItem(key);
    } catch (e) {
      console.warn(`[storage] Failed to remove ${key}`, e);
    }
  }
};

// Logger
export const logger = {
  info: (msg: string, ...args: unknown[]) => {
    console.log(`[INFO] [${new Date().toISOString()}] ${msg}`, ...args);
  },
  warn: (msg: string, ...args: unknown[]) => {
    console.warn(`[WARN] [${new Date().toISOString()}] ${msg}`, ...args);
  },
  error: (msg: string, ...args: unknown[]) => {
    console.error(`[ERROR] [${new Date().toISOString()}] ${msg}`, ...args);
  }
};

// Simple Result pattern
export type Result<T, E = Error> =
  | { ok: true; value: T }
  | { ok: false; error: E };

export function ok<T>(value: T): Result<T, never> {
  return { ok: true, value };
}

export function err<E>(error: E): Result<never, E> {
  return { ok: false, error };
}
