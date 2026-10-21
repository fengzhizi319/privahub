import React, { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react';

import { Modal, Button } from './index';

/** Modal 模态对话框文档。通过 isOpen 受控显隐。 */
const meta: Meta<typeof Modal> = {
  title: 'Components/Modal',
  component: Modal,
  tags: ['autodocs'],
};

export default meta;

type Story = StoryObj<typeof Modal>;

/**
 * 可交互演示组件：用本地 state 控制 Modal 的显隐。
 * 抽离为独立的 React 组件（而非在 render 函数内直接调用 hooks），
 * 以满足 react-hooks/rules-of-hooks 规则。
 */
const InteractiveModalDemo: React.FC = () => {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        打开对话框
      </Button>
      <Modal
        isOpen={open}
        onClose={() => setOpen(false)}
        title="示例对话框"
        footer={
          <>
            <Button variant="outline" onClick={() => setOpen(false)}>
              取消
            </Button>
            <Button variant="primary" onClick={() => setOpen(false)}>
              确定
            </Button>
          </>
        }
      >
        这是对话框的主体内容。
      </Modal>
    </>
  );
};

/**
 * 可交互示例：点击按钮打开对话框。
 * 由于 Modal 受 `isOpen` 控制，这里通过 render 渲染一个带本地状态的演示组件。
 */
export const Interactive: Story = {
  render: () => <InteractiveModalDemo />,
};
