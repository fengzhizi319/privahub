import { describe, it, expect, beforeEach } from 'vitest';

import { apiClient } from './client';
import { server } from '../../../test/mocks/server';
import { errorHandlers } from '../../../test/mocks/handlers';

/**
 * apiClient 集成测试（基于 MSW）。
 *
 * 与 `client.test.ts`（用 vi.mock 替换 api 模块的单元测试）不同，
 * 本文件不 mock 任何模块，而是让真实的 openapi-fetch 客户端发出请求，
 * 由 MSW 在网络层拦截并返回预设响应。因此能够覆盖完整的真实链路：
 *   请求构造 → 请求头注入（User-Token/Trace-Id）→ 网络往返 →
 *   响应解包 → Zod 运行时校验 → 结果归一化。
 *
 * MSW 服务器的生命周期由 `test/setup.ts` 统一管理（启动/重置/关闭）。
 */

beforeEach(() => {
  // 每个用例前清空存储，确保 token 相关断言互不干扰。
  localStorage.clear();
});

describe('apiClient 集成：登录链路', () => {
  it('成功登录时落盘 token 并返回用户上下文', async () => {
    const user = await apiClient.login('admin', 'correct-hash');

    expect(user.name).toBe('admin');
    expect(user.token).toBe('msw-token-123');
    // 验证 token 真正写入了 localStorage（真实副作用）。
    expect(localStorage.getItem('secretpad-token')).toBe('msw-token-123');
  });

  it('凭证错误时抛出后端返回的业务错误信息', async () => {
    await expect(apiClient.login('admin', 'wrong-hash')).rejects.toThrow(
      'invalid username or password'
    );
    // 登录失败不应写入 token。
    expect(localStorage.getItem('secretpad-token')).toBeNull();
  });
});

describe('apiClient 集成：节点列表归一化', () => {
  it('将后端 NodeVO 字段映射为前端 Node 模型', async () => {
    const nodes = await apiClient.getNodes();

    expect(nodes).toHaveLength(1);
    expect(nodes[0]).toMatchObject({
      nodeId: 'alice',
      nodeName: 'Alice Node',
      name: 'Alice Node',
      status: 'Ready',
      nodeStatus: 'Ready',
    });
  });
});

describe('apiClient 集成：登出清理', () => {
  it('登出后清除本地 token', async () => {
    localStorage.setItem('secretpad-token', 'to-be-cleared');

    await apiClient.logout();

    expect(localStorage.getItem('secretpad-token')).toBeNull();
  });
});

describe('apiClient 集成：Zod 运行时校验', () => {
  it('后端返回缺失必填字段的数据时抛出校验错误', async () => {
    // 临时覆盖默认 handler，返回非法（缺 projectId）的项目数据。
    server.use(errorHandlers.invalidProjectList);

    await expect(apiClient.listP2pProjects()).rejects.toThrow(
      /API schema validation failed/
    );
  });
});
