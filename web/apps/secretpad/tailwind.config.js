/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{js,ts,jsx,tsx}',
    '../../packages/design-system/src/**/*.{js,ts,jsx,tsx}',
    '../../packages/dag-next/src/**/*.{js,ts,jsx,tsx}',
  ],
  // 暗色模式同时支持 `.dark` 类与 `data-theme="dark"` 属性，
  // 与主题模块（shared/lib/theme）及 index.css 的令牌覆盖选择器保持一致。
  darkMode: ['class', '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        // 保留原有品牌色阶（兼容已有代码）。
        brand: {
          50: '#e6f4ff',
          100: '#bae0ff',
          500: '#1677ff',
          600: '#0958d9',
          700: '#003eb3',
        },
        // 语义色：直接引用 CSS 变量，随主题自动切换。
        // 用法示例：bg-surface-card / text-fg-primary / border-line-base。
        surface: {
          app: 'var(--surface-app)',
          card: 'var(--surface-card)',
          elevated: 'var(--surface-elevated)',
          input: 'var(--surface-input)',
          hover: 'var(--surface-hover)',
        },
        fg: {
          primary: 'var(--text-primary)',
          secondary: 'var(--text-secondary)',
          muted: 'var(--text-muted)',
          inverse: 'var(--text-inverse)',
        },
        line: {
          base: 'var(--border-base)',
          strong: 'var(--border-strong)',
        },
        accent: {
          DEFAULT: 'var(--brand-primary)',
          hover: 'var(--brand-primary-hover)',
          active: 'var(--brand-primary-active)',
          bg: 'var(--brand-primary-bg)',
        },
      },
      borderRadius: {
        // 将圆角令牌暴露为 Tailwind 工具类（rounded-token-md 等）。
        'token-sm': 'var(--radius-sm)',
        'token-md': 'var(--radius-md)',
        'token-lg': 'var(--radius-lg)',
      },
      boxShadow: {
        card: 'var(--shadow-card)',
        elevated: 'var(--shadow-elevated)',
      },
    },
  },
  plugins: [],
};
