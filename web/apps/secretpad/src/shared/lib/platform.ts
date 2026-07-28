import { useAuthStore } from '@/features/auth/model/auth-store';

export enum Platform {
  CENTER = 'CENTER',
  EDGE = 'EDGE',
  AUTONOMY = 'AUTONOMY',
  TEST = 'TEST',
  P2P = 'P2P',
}

export enum PadMode {
  TEE = 'TEE',
  MPC = 'MPC',
  ALL_IN_ONE = 'ALL-IN-ONE',
}

export interface AccessType {
  types?: Platform[];
  modes?: PadMode[];
}

export const EMBEDDED_NODES = ['alice', 'bob', 'tee'];

export function usePlatform() {
  const { user, platform } = useAuthStore();
  const platformType = (user?.platformType as Platform) || platform.platformType;
  const ownerId = user?.ownerId || platform.nodeId;
  const deployMode = (user?.deployMode as PadMode) || PadMode.ALL_IN_ONE;

  return {
    platformType,
    ownerId,
    deployMode,
    isCenter: platformType === Platform.CENTER,
    isEdge: platformType === Platform.EDGE,
    isAutonomy: platformType === Platform.AUTONOMY,
    isP2p: platformType === Platform.P2P || platformType === Platform.AUTONOMY,
    isEmbeddedNode: (nodeId?: string) => !!nodeId && EMBEDDED_NODES.includes(nodeId),
  };
}

export function useHasAccess(access: AccessType = {}): boolean {
  const { platformType, deployMode } = usePlatform();
  const types = access.types ?? [Platform.CENTER, Platform.EDGE, Platform.AUTONOMY];
  const modes = access.modes ?? [PadMode.ALL_IN_ONE, PadMode.MPC, PadMode.TEE];
  return types.includes(platformType) && modes.includes(deployMode);
}

export function useCanAccessEmbeddedNode(nodeId?: string): boolean {
  const { user } = useAuthStore();
  return (
    user?.platformType === Platform.CENTER &&
    user?.ownerType === 'CENTER' &&
    !!nodeId &&
    EMBEDDED_NODES.includes(nodeId)
  );
}
