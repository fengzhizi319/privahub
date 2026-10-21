import { describe, it, expect, beforeEach } from 'vitest';
import React from 'react';
import { renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  usePlatform,
  useHasAccess,
  useCanAccessEmbeddedNode,
  useCanWrite,
  Platform,
  PadMode,
  toPlatformContext,
  canWriteNode,
  supportsTee,
  resolveMyNodeId,
} from './platform';
import { useAuthStore } from '@/features/auth/model/auth-store';

function setPlatform(platformType: string, deployMode = 'ALL-IN-ONE', ownerType = 'CENTER', ownerId = 'kuscia-system') {
  useAuthStore.setState({
    user: {
      name: 'admin',
      ownerId,
      platformType,
      platformNodeId: ownerId,
      ownerType,
      deployMode,
    } as any,
    platform: { platformType: platformType as any, nodeId: ownerId },
  });
}

describe('platform guard', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('flags P2P platform type as isP2p', () => {
    setPlatform('P2P');
    const { result } = renderHook(() => usePlatform());
    expect(result.current.isP2p).toBe(true);
    expect(result.current.platformType).toBe(Platform.P2P);
  });

  it('treats AUTONOMY as P2P-capable', () => {
    setPlatform('AUTONOMY');
    const { result } = renderHook(() => usePlatform());
    expect(result.current.isP2p).toBe(true);
    expect(result.current.isAutonomy).toBe(true);
  });

  it('does not flag CENTER as P2P', () => {
    setPlatform('CENTER');
    const { result } = renderHook(() => usePlatform());
    expect(result.current.isP2p).toBe(false);
    expect(result.current.isCenter).toBe(true);
  });

  it('gates access by platform type via useHasAccess', () => {
    setPlatform('P2P');
    const { result } = renderHook(() => useHasAccess({ types: [Platform.CENTER] }));
    expect(result.current).toBe(false);
  });

  it('grants access when platform type matches', () => {
    setPlatform('CENTER');
    const { result } = renderHook(() =>
      useHasAccess({ types: [Platform.CENTER], modes: [PadMode.ALL_IN_ONE] }),
    );
    expect(result.current).toBe(true);
  });

  it('grants access for the user shape produced by apiClient.login', () => {
    // apiClient.login 映射出的用户（platformType='CENTER', deployMode='ALL-IN-ONE'）
    // 必须通过 AccessGuard，否则 /dag 等页面会退化为只读。
    setPlatform('CENTER', 'ALL-IN-ONE');
    const { result } = renderHook(() => useHasAccess({ types: [Platform.CENTER] }));
    expect(result.current).toBe(true);
  });

  it('normalizes an invalid deployMode to ALL-IN-ONE instead of denying everything', () => {
    // 回归：登录映射曾误把 deployMode 写成 'CENTER'，modes 校验恒为 false。
    setPlatform('CENTER', 'CENTER');
    const { result } = renderHook(() => useHasAccess({ types: [Platform.CENTER] }));
    expect(result.current).toBe(true);
    expect(renderHook(() => usePlatform()).result.current.deployMode).toBe(PadMode.ALL_IN_ONE);
  });

  it('useHasAccess defaults include every platform type (P2P and TEST too)', () => {
    for (const type of ['P2P', 'TEST', 'EDGE', 'AUTONOMY', 'CENTER']) {
      setPlatform(type);
      expect(renderHook(() => useHasAccess()).result.current).toBe(true);
    }
  });

  it('respects deployMode restrictions (TEE-only features hidden on MPC)', () => {
    setPlatform('CENTER', 'MPC');
    expect(renderHook(() => useHasAccess({ modes: [PadMode.TEE] })).result.current).toBe(false);
    expect(renderHook(() => usePlatform()).result.current.supportsTee).toBe(false);
    expect(supportsTee(toPlatformContext({ deployMode: 'TEE' }))).toBe(true);
  });

  it('flags EDGE accounts on CENTER', () => {
    setPlatform('CENTER', 'ALL-IN-ONE', 'EDGE', 'alice');
    const { result } = renderHook(() => usePlatform());
    expect(result.current.isEdgeAccountOnCenter).toBe(true);
    expect(result.current.isCenterAdmin).toBe(false);
  });

  it('allows embedded nodes only for CENTER admins', () => {
    setPlatform('CENTER');
    expect(renderHook(() => useCanAccessEmbeddedNode('alice')).result.current).toBe(true);
    expect(renderHook(() => useCanAccessEmbeddedNode('carol')).result.current).toBe(false);
    setPlatform('CENTER', 'ALL-IN-ONE', 'EDGE', 'alice');
    expect(renderHook(() => useCanAccessEmbeddedNode('bob')).result.current).toBe(false);
  });

  it('resolves the /p2p/my-node target per platform (never the CENTER platform id)', () => {
    const centerAdmin = toPlatformContext({ platformType: 'CENTER', ownerType: 'CENTER', ownerId: 'kuscia-system' });
    expect(resolveMyNodeId(centerAdmin)).toBeNull();
    expect(resolveMyNodeId(centerAdmin, 'kuscia-system')).toBeNull();
    expect(resolveMyNodeId(centerAdmin, 'carol')).toBeNull();
    expect(resolveMyNodeId(centerAdmin, 'alice')).toBe('alice');
    const edgeOnCenter = toPlatformContext({ platformType: 'CENTER', ownerType: 'EDGE', ownerId: 'alice' });
    expect(resolveMyNodeId(edgeOnCenter, 'bob')).toBe('alice');
    expect(resolveMyNodeId(toPlatformContext({ platformType: 'EDGE', ownerType: 'EDGE', ownerId: 'n1' }))).toBe('n1');
    expect(resolveMyNodeId(toPlatformContext({ platformType: 'AUTONOMY', ownerType: 'P2P', ownerId: 'inst-a' }), 'x')).toBe('inst-a');
    expect(resolveMyNodeId(toPlatformContext({ platformType: 'TEST', ownerId: 't' }), 'bob')).toBe('bob');
  });

  it('grants AUTONOMY write permission on every node of its institution (ownerId is the inst id)', () => {
    const autonomy = toPlatformContext({ platformType: 'AUTONOMY', ownerType: 'P2P', ownerId: 'inst-a' });
    // Backend DataResourceAuth NODE_ID: ownerId == nodeId || node.inst_id == ownerId.
    expect(canWriteNode(autonomy, 'inst-a')).toBe(true);
    expect(canWriteNode(autonomy, 'node-a1')).toBe(false);
    expect(canWriteNode(autonomy, 'node-a1', { instId: 'inst-a' })).toBe(true);
    expect(canWriteNode(autonomy, 'node-b1', { instId: 'inst-b' })).toBe(false);
    expect(canWriteNode(autonomy, 'node-a2', { instNodeIds: ['node-a1', 'node-a2'] })).toBe(true);
    expect(canWriteNode(autonomy, 'bob', { instNodeIds: ['node-a1'] })).toBe(false);
    expect(canWriteNode(autonomy)).toBe(true);
    // Institution membership lists only widen AUTONOMY; P2P/EDGE stay on their own node.
    const p2p = toPlatformContext({ platformType: 'P2P', ownerType: 'P2P', ownerId: 'alice' });
    expect(canWriteNode(p2p, 'bob', { instNodeIds: ['bob'] })).toBe(false);
    expect(canWriteNode(toPlatformContext({ platformType: 'CENTER', ownerType: 'CENTER' }), 'bob')).toBe(true);
    expect(canWriteNode(toPlatformContext({ platformType: 'CENTER', ownerType: 'EDGE', ownerId: 'alice' }), 'bob')).toBe(
      false,
    );
  });

  it('useCanWrite resolves AUTONOMY institution nodes via inst/node/list', async () => {
    setPlatform('AUTONOMY', 'ALL-IN-ONE', 'P2P', 'inst-a');
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(['my-inst-nodes'], [{ nodeId: 'node-a1', instId: 'inst-a' }]);
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    expect(renderHook(() => useCanWrite('node-a1'), { wrapper }).result.current).toBe(true);
    expect(renderHook(() => useCanWrite('node-x', 'inst-a'), { wrapper }).result.current).toBe(true);
    expect(renderHook(() => useCanWrite('bob'), { wrapper }).result.current).toBe(false);
  });
});
