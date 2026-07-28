import type { Meta, StoryObj } from '@storybook/react';

import { Badge } from './index';

/** Badge 状态徽标文档。用于展示任务/节点/路由等的运行状态。 */
const meta: Meta<typeof Badge> = {
  title: 'Components/Badge',
  component: Badge,
  tags: ['autodocs'],
  argTypes: {
    status: {
      control: 'select',
      options: ['success', 'processing', 'warning', 'error', 'default'],
    },
  },
};

export default meta;

type Story = StoryObj<typeof Badge>;

/** 成功状态。 */
export const Success: Story = {
  args: { status: 'success', children: 'Running' },
};

/** 进行中状态（圆点带脉冲动画）。 */
export const Processing: Story = {
  args: { status: 'processing', children: 'Processing' },
};

/** 告警状态。 */
export const Warning: Story = {
  args: { status: 'warning', children: 'Degraded' },
};

/** 错误状态。 */
export const Error: Story = {
  args: { status: 'error', children: 'Failed' },
};

/** 全部状态一览。 */
export const AllStatuses: Story = {
  render: () => (
    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
      <Badge status="success">Success</Badge>
      <Badge status="processing">Processing</Badge>
      <Badge status="warning">Warning</Badge>
      <Badge status="error">Error</Badge>
      <Badge status="default">Default</Badge>
    </div>
  ),
};
