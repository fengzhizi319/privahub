import { describe, it, expect, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { usePlatform, useHasAccess, Platform, PadMode } from './platform';
import { useAuthStore } from '@/features/auth/model/auth-store';

function setPlatform(platformType: string, deployMode = 'ALL-IN-ONE') {
  useAuthStore.setState({
    user: {
      name: 'admin',
      ownerId: 'kuscia-system',
      platformType,
      platformNodeId: 'kuscia-system',
      ownerType: 'CENTER',
      deployMode,
    } as any,
    platform: { platformType: platformType as any, nodeId: 'kuscia-system' },
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
});
