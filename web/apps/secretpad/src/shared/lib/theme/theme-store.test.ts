import { describe, it, expect, beforeEach, vi } from 'vitest';

import {
  resolveTheme,
  getStoredMode,
  applyTheme,
  getSystemTheme,
  useThemeStore,
} from './theme-store';

/**
 * theme-store.ts 单元测试。
 *
 * 覆盖主题机制的核心纯逻辑与 DOM 副作用：
 * - 模式解析（system → 系统偏好）。
 * - 持久化读取与非法值容错。
 * - 应用到 <html> 的 data-theme / .dark 类。
 * - store 的 setMode / toggle 行为与持久化。
 */

beforeEach(() => {
  localStorage.clear();
  // 清理 <html> 上的主题标记，保证用例隔离。
  document.documentElement.removeAttribute('data-theme');
  document.documentElement.classList.remove('dark');
  vi.restoreAllMocks();
});

describe('resolveTheme', () => {
  it('light / dark 模式原样返回', () => {
    expect(resolveTheme('light')).toBe('light');
    expect(resolveTheme('dark')).toBe('dark');
  });

  it('system 模式解析为系统偏好', () => {
    // mock matchMedia 返回偏好暗色。
    vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: true,
    } as MediaQueryList);
    expect(resolveTheme('system')).toBe('dark');
  });
});

describe('getSystemTheme', () => {
  it('matchMedia 命中暗色时返回 dark', () => {
    vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: true,
    } as MediaQueryList);
    expect(getSystemTheme()).toBe('dark');
  });

  it('未命中时返回 light', () => {
    vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: false,
    } as MediaQueryList);
    expect(getSystemTheme()).toBe('light');
  });
});

describe('getStoredMode', () => {
  it('无存储时回退为 system', () => {
    expect(getStoredMode()).toBe('system');
  });

  it('非法值回退为 system', () => {
    localStorage.setItem('secretpad-theme', 'neon');
    expect(getStoredMode()).toBe('system');
  });

  it('合法值原样返回', () => {
    localStorage.setItem('secretpad-theme', 'dark');
    expect(getStoredMode()).toBe('dark');
  });
});

describe('applyTheme', () => {
  it('dark 时设置 data-theme 并添加 .dark 类', () => {
    applyTheme('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('light 时设置 data-theme 并移除 .dark 类', () => {
    document.documentElement.classList.add('dark');
    applyTheme('light');
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });
});

describe('useThemeStore', () => {
  it('setMode 持久化并应用主题', () => {
    useThemeStore.getState().setMode('dark');

    expect(localStorage.getItem('secretpad-theme')).toBe('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(useThemeStore.getState().mode).toBe('dark');
    expect(useThemeStore.getState().resolved).toBe('dark');
  });

  it('toggle 在明暗间切换并脱离 system', () => {
    useThemeStore.getState().setMode('light');
    useThemeStore.getState().toggle();

    expect(useThemeStore.getState().mode).toBe('dark');
    expect(localStorage.getItem('secretpad-theme')).toBe('dark');
  });
});
