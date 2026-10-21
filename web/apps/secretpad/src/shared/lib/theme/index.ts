/**
 * 主题模块统一出口。
 * 对外暴露主题 Store、初始化函数及类型，供应用入口与 UI 组件使用。
 */
export {
  useThemeStore,
  initTheme,
  applyTheme,
  resolveTheme,
  getStoredMode,
  getSystemTheme,
  type ThemeMode,
  type ResolvedTheme,
} from './theme-store';
