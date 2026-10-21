import { createContext, useContext } from 'react';

/**
 * Node-context view (`/node/$nodeId/*`). Pages rendered inside the node layout
 * (data sources, data tables, cooperative nodes, results) read the node id
 * from here and scope their queries/writes to it instead of showing a node
 * picker. Outside the layout `nodeId` is undefined.
 */
export interface NodeContextValue {
  nodeId?: string;
  nodeName?: string;
  /** Node type from node/get (e.g. 'embedded' / 'tee'); tee nodes hide datasources. */
  nodeType?: string;
}

export const NodeCtx = createContext<NodeContextValue>({});

export function useNodeContext(): NodeContextValue {
  return useContext(NodeCtx);
}
