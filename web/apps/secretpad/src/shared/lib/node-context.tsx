import React from 'react';
import { NodeCtx, type NodeContextValue } from './use-node-context';

/** Provider of the node-context view; read it with `useNodeContext` (./use-node-context). */
export const NodeContextProvider: React.FC<{ value: NodeContextValue; children: React.ReactNode }> = ({ value, children }) => (
  <NodeCtx.Provider value={value}>{children}</NodeCtx.Provider>
);
