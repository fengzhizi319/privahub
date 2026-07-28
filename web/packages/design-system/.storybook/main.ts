import type { StorybookConfig } from '@storybook/react-vite';

/**
 * Storybook 主配置（@secretpad/design-system）。
 *
 * 说明：
 * - `stories`：声明 stories 文件的扫描范围（组件库源码内就近放置 *.stories.tsx）。
 * - `addons`：
 *   - essentials：控件 / 操作 / 文档 / 视口等常用面板。
 *   - @storybook/addon-themes：提供工具栏主题切换，配合明暗 Design Token 预览。
 * - `framework`：使用 react-vite 构建器，复用项目的 Vite 生态，启动快。
 * - `docs.autodocs`：为每个组件自动生成文档页（MDX 风格的 ArgsTable）。
 */
const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|tsx)'],
  addons: ['@storybook/addon-essentials', '@storybook/addon-themes'],
  framework: {
    name: '@storybook/react-vite',
    options: {},
  },
  docs: {
    autodocs: 'tag',
  },
};

export default config;
