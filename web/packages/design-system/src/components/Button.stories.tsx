import type { Meta, StoryObj } from '@storybook/react';

import { Button } from './index';

/**
 * Button 组件文档。
 *
 * Meta 描述组件级别的元信息：
 * - `title`：决定其在 Storybook 侧边栏的分组路径。
 * - `component`：关联组件以自动生成 ArgsTable（props 文档）。
 * - `tags: ['autodocs']`：启用自动文档页。
 */
const meta: Meta<typeof Button> = {
  title: 'Components/Button',
  component: Button,
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['primary', 'outline', 'danger', 'ghost', 'link'],
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
    },
    loading: { control: 'boolean' },
    disabled: { control: 'boolean' },
  },
};

export default meta;

/** 每个 Story 描述组件的一种状态；args 为该状态下的 props。 */
type Story = StoryObj<typeof Button>;

/** 主按钮（默认展示态）。 */
export const Primary: Story = {
  args: { variant: 'primary', children: 'Primary Button' },
};

/** 描边按钮。 */
export const Outline: Story = {
  args: { variant: 'outline', children: 'Outline Button' },
};

/** 危险按钮。 */
export const Danger: Story = {
  args: { variant: 'danger', children: 'Delete' },
};

/** 加载中状态。 */
export const Loading: Story = {
  args: { variant: 'primary', loading: true, children: 'Submitting' },
};

/** 尺寸对比（通过 render 自定义布局展示全部尺寸）。 */
export const Sizes: Story = {
  render: () => (
    <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
      <Button size="sm" variant="primary">
        Small
      </Button>
      <Button size="md" variant="primary">
        Medium
      </Button>
      <Button size="lg" variant="primary">
        Large
      </Button>
    </div>
  ),
};
