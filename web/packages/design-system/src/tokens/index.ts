/**
 * SecretPad Design Tokens
 *
 * 说明：
 * - 运行时的「主题化」颜色/圆角/阴影由 CSS 自定义属性（见应用 index.css）驱动，
 *   会随明暗主题自动切换。这里的 `semantic` 段给出语义令牌与 CSS 变量名的映射，
 *   供 JS/TS 侧（如内联样式、图表配色）按名引用，保证与 CSS 令牌同源。
 * - `colors`/`spacing`/`borderRadius`/`typography` 为静态参考值（亮色基准），
 *   保留以兼容既有引用。
 */
export const tokens = {
  colors: {
    primary: '#1677ff',
    primaryHover: '#4096ff',
    primaryBg: '#e6f4ff',
    success: '#52c41a',
    successBg: '#f6ffed',
    warning: '#faad14',
    warningBg: '#fffbe6',
    error: '#ff4d4f',
    errorBg: '#fff2f0',
    purple: '#722ed1',
    purpleBg: '#f9f0ff',
    cyan: '#13c2c2',
    cyanBg: '#e6fffb',
  },
  /**
   * 语义令牌 → CSS 变量名映射。
   * 使用示例：`style={{ background: `var(${tokens.semantic.surface.card})` }}`。
   * 这些变量在 index.css 中按明暗主题分别定义，故能自动换肤。
   */
  semantic: {
    brand: {
      primary: '--brand-primary',
      primaryHover: '--brand-primary-hover',
      primaryActive: '--brand-primary-active',
      primaryBg: '--brand-primary-bg',
    },
    surface: {
      app: '--surface-app',
      card: '--surface-card',
      elevated: '--surface-elevated',
      input: '--surface-input',
      hover: '--surface-hover',
    },
    text: {
      primary: '--text-primary',
      secondary: '--text-secondary',
      muted: '--text-muted',
      inverse: '--text-inverse',
    },
    border: {
      base: '--border-base',
      strong: '--border-strong',
    },
    status: {
      success: '--status-success',
      successBg: '--status-success-bg',
      warning: '--status-warning',
      warningBg: '--status-warning-bg',
      error: '--status-error',
      errorBg: '--status-error-bg',
    },
  },
  spacing: {
    xs: '4px',
    sm: '8px',
    md: '16px',
    lg: '24px',
    xl: '32px',
  },
  borderRadius: {
    sm: '4px',
    md: '8px',
    lg: '12px',
    full: '9999px',
  },
  typography: {
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'PingFang SC', sans-serif",
    fontSizeSm: '12px',
    fontSizeBase: '14px',
    fontSizeLg: '16px',
    fontSizeXl: '20px',
  }
};
