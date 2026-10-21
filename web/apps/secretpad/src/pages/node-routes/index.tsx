import React from 'react';
import { CooperativeNodeList } from '@/features/cooperative-node/cooperative-node-list';
import { useNodeContext } from '@/shared/lib/use-node-context';

/**
 * 合作节点（节点路由）页。
 *
 * - 独立路由 `/node-routes`：CENTER 管理员可选择节点查看；EDGE / AUTONOMY 使用自己的 ownerId。
 * - 节点上下文 `/node/$nodeId/cooperative-nodes`：固定为该节点，隐藏节点选择。
 */
export const NodeRoutesPage: React.FC = () => {
  const { nodeId } = useNodeContext();
  return <CooperativeNodeList ownerNodeId={nodeId} compact={!!nodeId} />;
};
