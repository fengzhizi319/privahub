/**
 * 认证状态仓库（Zustand）。
 *
 * 职责：
 * - 维护当前登录用户、平台上下文（CENTER/P2P）与认证状态；
 * - 封装登录（密码先做 SHA-256 哈希再传输）、登出与状态重水合（rehydrate）。
 *
 * 关注点分离：主题（theme）相关状态已迁移至 `shared/lib/theme`，
 * 本仓库仅专注于认证与用户上下文。
 */
import { create } from 'zustand';
import { sha256 } from '@secretpad/utils';
import type { User, Platform } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';

/** 认证状态结构。 */
interface AuthState {
  /** 当前登录用户；未登录为 null。 */
  user: User | null;
  /** 平台上下文：决定后续接口走 CENTER 还是 P2P 分支。 */
  platform: Platform;
  /** 是否已认证（以本地是否存在 token 为准）。 */
  isAuthenticated: boolean;
  /** 从 localStorage 重新读取用户/token，用于刷新页面后恢复会话。 */
  rehydrate: () => void;
  /** 登录：密码哈希后调用后端接口，成功后落盘用户信息。 */
  login: (name: string, password: string) => Promise<void>;
  /** 登出：调用后端接口并清理本地凭证。 */
  logout: () => Promise<void>;
}

/** 从 localStorage 安全读取已持久化的用户信息，解析失败时返回 null。 */
function getStoredUser(): User | null {
  try {
    const raw = localStorage.getItem('secretpad-user');
    return raw ? (JSON.parse(raw) as User) : null;
  } catch {
    return null;
  }
}

/** 根据用户信息推导平台上下文，缺失时回退为 CENTER。 */
function buildPlatform(user: User | null): Platform {
  return {
    platformType: (user?.platformType as Platform['platformType']) || 'CENTER',
    nodeId: user?.platformNodeId || user?.ownerId || '',
  };
}

export const useAuthStore = create<AuthState>((set) => ({
  user: getStoredUser(),
  platform: buildPlatform(getStoredUser()),
  isAuthenticated: !!localStorage.getItem('secretpad-token'),

  rehydrate: () => {
    const user = getStoredUser();
    set({
      user,
      platform: buildPlatform(user),
      isAuthenticated: !!localStorage.getItem('secretpad-token'),
    });
  },

  login: async (name: string, password: string) => {
    // 密码不以明文传输：先做 SHA-256 哈希，再交由后端校验。
    const passwordHash = await sha256(password);
    const user = await apiClient.login(name, passwordHash);
    // 登录成功后持久化用户信息，供刷新页面时 rehydrate 使用。
    localStorage.setItem('secretpad-user', JSON.stringify(user));
    set({
      user,
      isAuthenticated: true,
      platform: {
        platformType: (user.platformType as Platform['platformType']) || 'CENTER',
        nodeId: user.platformNodeId || user.ownerId || '',
      },
    });
  },

  logout: async () => {
    await apiClient.logout();
    // 清理本地凭证与用户信息，并重置认证状态。
    localStorage.removeItem('secretpad-token');
    localStorage.removeItem('secretpad-user');
    set({ user: null, isAuthenticated: false });
  },
}));
