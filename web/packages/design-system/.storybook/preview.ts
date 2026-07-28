import type { Preview } from '@storybook/react';
import { withThemeByClassName } from '@storybook/addon-themes';

// 引入设计令牌（CSS 变量），使 Storybook 预览能正确渲染明暗主题。
import '../src/tokens/tokens.css';

/**
 * Storybook 预览配置（@secretpad/design-system）。
 *
 * - 通过 @storybook/addon-themes 的 withThemeByClassName 装饰器，
 *   在工具栏提供「Light / Dark」切换：其本质是在 <html> 上增删 `dark` 类，
 *   与 tokens.css 的暗色覆盖选择器（.dark）一致，从而实时换肤。
 * - 默认主题为 light。
 */
const preview: Preview = {
  parameters: {
    controls: {
      // 自动匹配组件 props 生成控件，便于交互式调试。
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    backgrounds: { disable: true },
  },
  decorators: [
    withThemeByClassName({
      themes: {
        light: '',
        dark: 'dark',
      },
      defaultTheme: 'light',
    }),
  ],
};

export default preview;
