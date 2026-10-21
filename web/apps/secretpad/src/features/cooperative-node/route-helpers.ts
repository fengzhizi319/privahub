import type { NodeRouterJava } from '@secretpad/api-client';
import { getProtocol, stripProtocol, type NetProtocol } from './auth-code';

/** Route between two built-in (alice/bob/tee) nodes: read-only in the UI. */
export const isEmbeddedRoute = (r?: NodeRouterJava) => r?.srcNode?.type === 'embedded' && r?.dstNode?.type === 'embedded';

/**
 * Build the nodeRoute/update body (legacy edit-modal): P2P edits the
 * cooperative (src) address, CENTER/EDGE edits the own (dst) address; the
 * other side keeps its current value.
 */
export function buildRouteUpdate(route: NodeRouterJava, p2p: boolean, protocol: NetProtocol, address: string) {
  const edited = `${protocol}${address}`;
  const keepDst = `${getProtocol(route.dstNetAddress || route.dstNode?.netAddress, route.dstNode?.protocol)}${stripProtocol(
    route.dstNetAddress || route.dstNode?.netAddress,
  )}`;
  const keepSrc = `${getProtocol(route.srcNetAddress, route.srcNode?.protocol)}${stripProtocol(route.srcNetAddress)}`;
  return {
    routerId: route.routeId || '',
    srcNetAddress: p2p ? edited : keepSrc,
    dstNetAddress: p2p ? keepDst : edited,
    routeType: route.routeType,
  };
}
