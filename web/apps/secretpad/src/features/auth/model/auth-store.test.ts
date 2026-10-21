import { describe, it, expect, vi, beforeEach } from 'vitest';

const { login, getUser, logout } = vi.hoisted(() => ({ login: vi.fn(), getUser: vi.fn(), logout: vi.fn() }));

vi.mock('@secretpad/api-client', async (orig) => {
  const actual = await orig<typeof import('@secretpad/api-client')>();
  return { ...actual, apiClient: { login, getUser, logout } };
});

import { useAuthStore } from './auth-store';
import { sha256, sm3 } from '@secretpad/utils';

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
});

describe('auth store', () => {
  it('sends sha256 + sm3 hashes and enriches the user from user/get', async () => {
    login.mockResolvedValue({ name: 'admin', token: '', ownerId: 'kuscia-system', platformType: 'CENTER', ownerType: 'CENTER', deployMode: 'ALL-IN-ONE' });
    getUser.mockResolvedValue({ name: 'admin', platformType: 'AUTONOMY', ownerId: 'inst-a', ownerType: 'P2P', deployMode: 'MPC' });

    const user = await useAuthStore.getState().login('admin', 'Abcdefg1');

    expect(login).toHaveBeenCalledWith('admin', await sha256('Abcdefg1'), sm3('Abcdefg1'));
    expect(user).toMatchObject({ platformType: 'AUTONOMY', ownerId: 'inst-a', deployMode: 'MPC', token: '' });
    expect(useAuthStore.getState().isAuthenticated).toBe(true);
    expect(useAuthStore.getState().platform).toEqual({ platformType: 'AUTONOMY', nodeId: 'inst-a' });
    expect(JSON.parse(localStorage.getItem('secretpad-user') || '{}').deployMode).toBe('MPC');
    // The session is an HttpOnly cookie: nothing token-like is persisted.
    expect(localStorage.getItem('secretpad-token')).toBeNull();
  });

  it('skips user/get for a mustChangePassword session and keeps the flag', async () => {
    login.mockResolvedValue({ name: 'admin', token: '', ownerId: 'k', platformType: 'CENTER', mustChangePassword: true });
    const user = await useAuthStore.getState().login('admin', 'Abcdefg1');
    expect(user.mustChangePassword).toBe(true);
    expect(getUser).not.toHaveBeenCalled();
    expect(JSON.parse(localStorage.getItem('secretpad-user') || '{}').mustChangePassword).toBe(true);
    useAuthStore.getState().clearMustChangePassword();
    expect(useAuthStore.getState().user?.mustChangePassword).toBeUndefined();
    expect(JSON.parse(localStorage.getItem('secretpad-user') || '{}').mustChangePassword).toBeUndefined();
  });

  it('keeps the login user when user/get fails', async () => {
    login.mockResolvedValue({ name: 'u', token: 't', ownerId: 'alice', platformType: 'EDGE', ownerType: 'EDGE', deployMode: 'TEE' });
    getUser.mockRejectedValue(new Error('boom'));
    const user = await useAuthStore.getState().login('u', 'p');
    expect(user).toMatchObject({ platformType: 'EDGE', deployMode: 'TEE' });
  });

  it('refreshUser updates platform context on app load', async () => {
    localStorage.setItem('secretpad-user', JSON.stringify({ name: 'a', token: '', ownerId: 'x', platformType: 'CENTER' }));
    useAuthStore.getState().rehydrate();
    getUser.mockResolvedValue({ platformType: 'EDGE', ownerId: 'alice', ownerType: 'EDGE', deployMode: 'TEE', name: 'a' });
    await useAuthStore.getState().refreshUser();
    expect(useAuthStore.getState().user).toMatchObject({ platformType: 'EDGE', ownerId: 'alice', deployMode: 'TEE' });
  });

  it('logout clears credentials even if the API fails', async () => {
    localStorage.setItem('secretpad-user', JSON.stringify({ name: 'a', token: '', ownerId: 'x', platformType: 'CENTER' }));
    logout.mockRejectedValue(new Error('offline'));
    await expect(useAuthStore.getState().logout()).rejects.toThrow('offline');
    expect(localStorage.getItem('secretpad-user')).toBeNull();
    expect(useAuthStore.getState().isAuthenticated).toBe(false);
  });
});
