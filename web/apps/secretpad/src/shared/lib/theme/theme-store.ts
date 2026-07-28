import { create } from 'zustand';

/**
 * 主题管理模块（Design System · 主题切换机制）。
 *
 * 设计目标：
 * 1. **关注点分离**：将主题状态从 auth-store 中剥离，成为独立的基础设施。
 * 2. **三态模式**：支持 `light` / `dark` / `system`（跟随系统偏好）。
 * 3. **持久化**：用户选择写入 localStorage，刷新后保持。
 * 4. **防闪烁（FOUC）**：提供 `initTheme()` 在应用渲染前同步应用主题，
 *    避免「先亮后暗」的跳变。
 * 5. **响应系统变化**：当模式为 `system` 时，监听系统配色变化实时切换。
 *
 * 落地方式：在 <html> 上同步设置 `data-theme` 属性与 `.dark` 类，
 * 与 `index.css` 的令牌覆盖选择器、Tailwind 的 darkMode 配置保持一致。
 */

/** 用户可选择的主题模式。 */
export type ThemeMode = 'light' | 'dark' | 'system';

/** 实际渲染出的主题（system 会被解析为具体的 light/dark）。 */
export type ResolvedTheme = 'light' | 'dark';

/** localStorage 中持久化主题模式的键名。 */
const STORAGE_KEY = 'secretpad-theme';

interface ThemeState {
  /** 用户选择的模式（可能是 system）。 */
  mode: ThemeMode;
  /** 解析后的实际主题（用于 UI 展示当前明暗）。 */
  resolved: ResolvedTheme;
  /** 设置主题模式并立即生效 + 持久化。 */
  setMode: (mode: ThemeMode) => void;
  /** 在 light / dark 之间快捷切换（system 视为当前 resolved 取反）。 */
  toggle: () => void;
}

/**
 * 读取系统当前配色偏好。
 * 在不支持 matchMedia 的环境（如 SSR / 旧 jsdom）下回退为 light。
 */
export function getSystemTheme(): ResolvedTheme {
  if (typeof window === 'undefined' || !window.matchMedia) return 'light';
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

/**
 * 从 localStorage 读取已持久化的主题模式。
 * 非法 / 缺失时回退为 system（跟随系统，符合多数用户预期）。
 */
export function getStoredMode(): ThemeMode {
  if (typeof localStorage === 'undefined') return 'system';
  const stored = localStorage.getItem(STORAGE_KEY);
  return stored === 'light' || stored === 'dark' || stored === 'system'
    ? stored
    : 'system';
}

/**
 * 将「模式」解析为「实际主题」：system 时取系统偏好，否则取自身。
 */
export function resolveTheme(mode: ThemeMode): ResolvedTheme {
  return mode === 'system' ? getSystemTheme() : mode;
}

/**
 * 把解析后的主题应用到 <html>：同步设置 data-theme 属性与 .dark 类。
 * 同时设置两者是为了兼容两种消费方（CSS 属性选择器 / Tailwind class 策略）。
 */
export function applyTheme(resolved: ResolvedTheme): void {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  root.setAttribute('data-theme', resolved);
  root.classList.toggle('dark', resolved === 'dark');
}

/**
 * 应用启动时调用一次：读取持久化模式并立即应用，防止首屏闪烁。
 * 应在 ReactDOM.render 之前（main.tsx 顶层）同步执行。
 *
 * @returns 初始解析后的主题，供需要时引用。
 */
export function initTheme(): ResolvedTheme {
  const mode = getStoredMode();
  const resolved = resolveTheme(mode);
  applyTheme(resolved);

  // 当模式为 system 时，监听系统配色变化，实时重新应用。
  if (mode === 'system' && typeof window !== 'undefined' && window.matchMedia) {
    const mql = window.matchMedia('(prefers-color-scheme: dark)');
    mql.addEventListener('change', (e) => {
      // 仅当仍处于 system 模式时跟随系统（避免覆盖用户后续的手动选择）。
      if (getStoredMode() === 'system') {
        applyTheme(e.matches ? 'dark' : 'light');
      }
    });
  }

  return resolved;
}

/**
 * 主题 Zustand Store。
 * 组件通过 `useThemeStore` 订阅 mode/resolved，并调用 setMode/toggle 切换。
 */
export const useThemeStore = create<ThemeState>((set) => ({
  mode: getStoredMode(),
  resolved: resolveTheme(getStoredMode()),

  setMode: (mode) => {
    const resolved = resolveTheme(mode);
    // 持久化用户选择。
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, mode);
    }
    applyTheme(resolved);
    set({ mode, resolved });
  },

  toggle: () => {
    // 以当前 resolved 取反作为明确的 light/dark 选择（脱离 system）。
    const current = resolveTheme(getStoredMode());
    const next: ThemeMode = current === 'dark' ? 'light' : 'dark';
    const resolved = resolveTheme(next);
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, next);
    }
    applyTheme(resolved);
    set({ mode: next, resolved });
  },
}));
