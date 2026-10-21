import type { Meta, StoryObj } from '@storybook/react';

import { Card, Button } from './index';

/** Card 容器卡片文档。提供标题栏 + 内容区的通用布局容器。 */
const meta: Meta<typeof Card> = {
  title: 'Components/Card',
  component: Card,
  tags: ['autodocs'],
};

export default meta;

type Story = StoryObj<typeof Card>;

/** 基础卡片（仅内容）。 */
export const Basic: Story = {
  args: { children: '这是一段卡片内容。' },
};

/** 带标题与右侧操作的卡片。 */
export const WithTitleAndExtra: Story = {
  args: {
    title: '节点概览',
    extra: <Button size="sm">刷新</Button>,
    children: '卡片主体内容区域，可放置任意元素。',
  },
};
