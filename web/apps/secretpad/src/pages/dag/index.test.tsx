import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';

/**
 * /dag 页面级回归测试。
 *
 * 覆盖两条曾导致「组件无法拖到画布」的真实回归：
 * 1. 拖入节点后，onNodeSelect 触发页面重渲染；若 initialNodes 未 memoize，
 *    dag-next 的同步 effect 会立刻把画布重置回服务端数据，节点瞬间消失。
 * 2. 画布应在服务端图数据更新时（如 refetch）正确跟随同步。
 *
 * 另见 packages/dag-next/src/index.test.tsx（画布自身的拖拽契约）与
 * shared/lib/platform.test.ts（AccessGuard 放行，readOnly 不再误触发）。
 */

// ---- 依赖 mock：路由查询参数、API 客户端、重型子特性 -------------------------

const routeSearch = vi.hoisted(() => ({ value: {} as Record<string, string> }));
vi.mock('@tanstack/react-router', () => ({
  useSearch: () => routeSearch.value,
  useNavigate: () => vi.fn(),
}));

const mockApi = vi.hoisted(() => ({
  getProjects: vi.fn(),
  getGraphs: vi.fn(),
  createGraph: vi.fn(),
  deleteGraph: vi.fn(),
  renameGraph: vi.fn(),
  getModels: vi.fn(),
  downloadData: vi.fn(),
}));

const mockJava = vi.hoisted(() => ({
  getProjectDetailJava: vi.fn(),
  listComponentsJava: vi.fn(),
  getComponentI18nJava: vi.fn(),
  getGraphDetailJava: vi.fn(),
  batchComponentsJava: vi.fn(),
  refreshGraphNodeMaxIndexJava: vi.fn(),
  fullUpdateGraphJava: vi.fn(),
  updateGraphNodeJava: vi.fn(),
  listGraphNodeStatusJava: vi.fn(),
  startGraphJava: vi.fn(),
  stopGraphNodeJava: vi.fn(),
  getGraphNodeLogsJava: vi.fn(),
  getCloudLogsJava: vi.fn(),
  getGraphNodeOutputJava: vi.fn(),
  getProjectDatatableColumnsJava: vi.fn(),
  createGraphJava: vi.fn(),
}));

vi.mock('@secretpad/api-client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@secretpad/api-client')>();
  return { ...actual, apiClient: mockApi, ...mockJava };
});

vi.mock('../../features/dag-templates', () => ({
  useTemplateWizard: () => ({ open: vi.fn() }),
  TemplateWizard: () => null,
}));

vi.mock('../../features/model-pack', () => ({
  ModelPackModal: () => null,
}));

vi.mock('../../features/scheduled-task-from-dag', () => ({
  ScheduledTaskFromDagModal: () => null,
}));

// 被测页面（在 mock 之后导入）
import { DAGPage } from './index';
import { I18nProvider } from '../../shared/lib/i18n';

// ---- 测试数据 ---------------------------------------------------------------

import { normalizeGraphDetail } from '@secretpad/api-client/src/ext/graph';

const componentVO = {
  code_name: 'ml.train/ss_sgd_train',
  name: 'ss_sgd_train',
  domain: 'ml.train',
  version: '1.0.0',
  desc: 'Secure SGD train',
};

let graphDetailData: { nodes: any[]; edges: any[] };

function setupApiMock() {
  mockApi.getProjects.mockResolvedValue([{ projectId: 'p1', projectName: 'Demo Project', nodes: [] }]);
  mockApi.getGraphs.mockResolvedValue([{ graphId: 'g1', name: 'Graph 1' }]);
  mockJava.getProjectDetailJava.mockResolvedValue({ projectId: 'p1', computeMode: 'MPC', nodes: [] });
  mockJava.listComponentsJava.mockResolvedValue([{ app: 'secretflow', ...componentVO }]);
  mockJava.getComponentI18nJava.mockResolvedValue({});
  mockJava.getGraphDetailJava.mockImplementation(() => Promise.resolve(normalizeGraphDetail(graphDetailData)));
  mockJava.batchComponentsJava.mockResolvedValue({
    'ml.train/ss_sgd_train': {
      domain: 'ml.train',
      name: 'ss_sgd_train',
      version: '1.0.0',
      inputs: [{ name: 'input_ds', types: ['sf.table.vertical'] }],
      outputs: [{ name: 'output_model', types: ['sf.model.ss_sgd'] }, { name: 'report', types: ['sf.report'] }],
      attrs: [],
    },
  });
  mockJava.refreshGraphNodeMaxIndexJava.mockResolvedValue(32);
  mockJava.fullUpdateGraphJava.mockResolvedValue(undefined);
  mockJava.updateGraphNodeJava.mockResolvedValue(undefined);
  mockJava.listGraphNodeStatusJava.mockResolvedValue({ finished: true, nodes: [] });
}

function createDataTransfer() {
  const store: Record<string, string> = {};
  return {
    dropEffect: 'none',
    effectAllowed: 'all',
    setData: (type: string, val: string) => {
      store[type] = val;
    },
    getData: (type: string) => store[type] ?? '',
    clearData: () => {
      Object.keys(store).forEach((k) => delete store[k]);
    },
  };
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <DAGPage />
      </I18nProvider>
    </QueryClientProvider>,
  );
  return { queryClient, ...utils };
}

function getCanvas(container: HTMLElement): HTMLElement {
  const svg = container.querySelector('svg.pointer-events-none');
  if (!svg || !svg.parentElement) throw new Error('canvas not found');
  return svg.parentElement as HTMLElement;
}

function dropOnCanvas(canvas: HTMLElement, dataTransfer: ReturnType<typeof createDataTransfer>) {
  fireEvent.dragOver(canvas, { dataTransfer });
  const dropEvent = new MouseEvent('drop', {
    bubbles: true,
    cancelable: true,
    clientX: 300,
    clientY: 200,
  });
  Object.defineProperty(dropEvent, 'dataTransfer', { value: dataTransfer });
  fireEvent(canvas, dropEvent);
}

const counterText = (text: string) => screen.queryByText(new RegExp(`^${text}$`));

// ---- 用例 -------------------------------------------------------------------

describe('/dag 页面：组件拖拽构建任务', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    graphDetailData = { nodes: [], edges: [] };
    setupApiMock();
  });

  // 页面底部的组件清单 Card 也会渲染组件名文本，与工作区组件库产生同名干扰，
  // 因此一律以工作区顶栏的节点/边计数作为「画布已就绪」的锚点。
  const waitWorkspaceReady = async () => {
    await waitFor(() => expect(counterText('0 nodes · 0 edges')).toBeTruthy());
    // 组件列表（component/list）异步加载，组件库条目出现后才能拖拽。
    await waitFor(() => expect(document.querySelector('[draggable="true"]')).toBeTruthy());
    // 还必须等图详情查询真正落地：其 resolve 会触发一次 server → canvas 全量同步，
    // 若在途时拖入节点，节点会被这次同步冲掉（与生产环境同一语义：服务端为准）。
    await waitFor(() => expect(mockJava.getGraphDetailJava).toHaveBeenCalled());
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  };

  it('画布以可编辑模式渲染（AccessGuard 放行，非只读回退）', async () => {
    const { container } = renderPage();
    // 只读回退不渲染 Save/Run 按钮；可编辑模式必须出现
    await waitWorkspaceReady();
    expect(screen.getByRole('button', { name: /Save/ })).toBeTruthy();
    const item = container.querySelector('[draggable="true"]');
    expect(item?.getAttribute('draggable')).toBe('true');
  });

  it('拖放组件到画布后节点出现且稳定存在（不因页面重渲染被重置）', async () => {
    const { container } = renderPage();
    await waitWorkspaceReady();
    const canvas = getCanvas(container);

    const item = container.querySelector('[draggable="true"]') as HTMLElement;
    const dt = createDataTransfer();
    fireEvent.dragStart(item, { dataTransfer: dt });
    dropOnCanvas(canvas, dt);

    // 节点落上画布
    await waitFor(() => expect(counterText('1 nodes · 0 edges')).toBeTruthy());

    // 回归核心：拖放会触发 onNodeSelect → 页面重渲染。
    // 修复前 initialNodes 每次渲染都是新引用，dag-next 的同步 effect
    // 会把画布重置回服务端空图 —— 等待若干宏任务后节点必须仍然存在。
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(counterText('1 nodes · 0 edges')).toBeTruthy();
    expect(within(canvas).getByText('ss_sgd_train')).toBeTruthy();
  });

  it('点击组件也可添加节点，多个节点可共存', async () => {
    const { container } = renderPage();
    await waitWorkspaceReady();
    const canvas = getCanvas(container);

    // 添加节点后画布上会出现同名文本，因此始终通过 draggable 属性定位组件库条目；
    // 点击须落在条目内部的文本上（onClick 挂在内层 div，事件自下而上冒泡触发）。
    const clickPaletteItem = () => {
      const item = container.querySelector('[draggable="true"]') as HTMLElement;
      fireEvent.click(within(item).getByText('ss_sgd_train'));
    };
    clickPaletteItem();
    await waitFor(() => expect(counterText('1 nodes · 0 edges')).toBeTruthy());

    clickPaletteItem();
    await waitFor(() => expect(counterText('2 nodes · 0 edges')).toBeTruthy());
    expect(within(canvas).getAllByText('ss_sgd_train')).toHaveLength(2);
  });

  it('服务端图数据更新时画布跟随同步（refetch 方向：server → canvas）', async () => {
    const { queryClient } = renderPage();
    await waitWorkspaceReady();

    // 服务端出现一个新节点（例如他处保存后重新拉取）
    graphDetailData = {
      nodes: [
        {
          graphNodeId: 'server-node-1',
          codeName: 'ml.train/ss_sgd_train',
          label: 'ServerNode',
          x: 120,
          y: 160,
          status: 'SUCCEED',
        },
      ],
      edges: [],
    };
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ['graph-detail'] });
    });

    await waitFor(() => expect(counterText('1 nodes · 0 edges')).toBeTruthy());
    expect(screen.getByText('ServerNode')).toBeTruthy();
  });
});

describe('/dag 页面：节点 ID、自动保存与合并', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    graphDetailData = { nodes: [], edges: [] };
    setupApiMock();
  });

  it('新增节点 ID 取 graph/node/max_index（${graphId}-node-${index}），并防抖自动保存为 graph/update', async () => {
    const { container } = renderPage();
    await waitFor(() => expect(counterText('0 nodes · 0 edges')).toBeTruthy());
    await waitFor(() => expect(mockJava.getGraphDetailJava).toHaveBeenCalled());
    await waitFor(() => expect(container.querySelector('[draggable="true"]')).toBeTruthy());
    const item = container.querySelector('[draggable="true"]') as HTMLElement;
    fireEvent.click(within(item).getByText('ss_sgd_train'));
    await waitFor(() => expect(counterText('1 nodes · 0 edges')).toBeTruthy());
    expect(mockJava.refreshGraphNodeMaxIndexJava).toHaveBeenCalledWith('p1', 'g1', undefined);
    await waitFor(() => expect(mockJava.fullUpdateGraphJava).toHaveBeenCalled(), { timeout: 3000 });
    const body = mockJava.fullUpdateGraphJava.mock.calls.at(-1)![0];
    expect(body.nodes[0]).toMatchObject({
      graphNodeId: 'g1-node-33',
      codeName: 'ml.train/ss_sgd_train',
      outputs: ['g1-node-33-output-0', 'g1-node-33-output-1'],
    });
  });

  it('本地有未保存改动时，服务端刷新不会冲掉本地新增节点', async () => {
    const { container, queryClient } = renderPage();
    await waitFor(() => expect(counterText('0 nodes · 0 edges')).toBeTruthy());
    await waitFor(() => expect(mockJava.getGraphDetailJava).toHaveBeenCalled());
    await waitFor(() => expect(container.querySelector('[draggable="true"]')).toBeTruthy());
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
    // 保存挂起：模拟慢请求，保证刷新发生时仍是脏状态。
    mockJava.fullUpdateGraphJava.mockImplementation(() => new Promise(() => undefined));
    const item = container.querySelector('[draggable="true"]') as HTMLElement;
    fireEvent.click(within(item).getByText('ss_sgd_train'));
    await waitFor(() => expect(counterText('1 nodes · 0 edges')).toBeTruthy());
    graphDetailData = { nodes: [], edges: [] };
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ['graph-detail'] });
    });
    expect(counterText('1 nodes · 0 edges')).toBeTruthy();
  });

  it('运行中节点状态来自 graph/node/status 覆盖层', async () => {
    graphDetailData = {
      nodes: [{ graphNodeId: 'g1-node-1', codeName: 'ml.train/ss_sgd_train', label: 'Train', x: 10, y: 10, status: 'STAGING' }],
      edges: [],
    };
    mockJava.listGraphNodeStatusJava.mockResolvedValue({ finished: false, nodes: [{ graphNodeId: 'g1-node-1', status: 'RUNNING' }] });
    renderPage();
    await waitFor(() => expect(screen.queryAllByText('运行中').length + screen.queryAllByText('Running').length).toBeGreaterThan(0));
  });
});


it('同一路由选择新画布时刷新节点，加载期间不保留上一张图', async () => {
  routeSearch.value = { projectId: 'p1', graphId: 'g1' };
  setupApiMock();
  mockApi.getGraphs.mockResolvedValue([{ graphId: 'g1', name: 'Graph 1' }, { graphId: 'g2', name: 'Graph 2' }]);
  let resolveNext!: (value: unknown) => void;
  mockJava.getGraphDetailJava.mockImplementation((_project: string, id: string) => id === 'g1'
    ? Promise.resolve(normalizeGraphDetail({ nodes: [{ graphNodeId: 'g1-node-1', codeName: 'ml.train/ss_sgd_train', label: 'PreviousCanvas', x: 10, y: 10 }], edges: [] }))
    : new Promise(resolve => { resolveNext = resolve; }));
  const { queryClient, rerender } = renderPage();
  await screen.findByText('PreviousCanvas');
  routeSearch.value = { projectId: 'p1', graphId: 'g2' };
  rerender(<QueryClientProvider client={queryClient}><I18nProvider><DAGPage /></I18nProvider></QueryClientProvider>);
  await waitFor(() => expect(mockJava.getGraphDetailJava).toHaveBeenCalledWith('p1', 'g2'));
  expect(screen.queryByText('PreviousCanvas')).toBeNull();
  await act(async () => resolveNext(normalizeGraphDetail({ nodes: [{ graphNodeId: 'g2-node-1', codeName: 'ml.train/ss_sgd_train', label: 'NextCanvas', x: 10, y: 10 }], edges: [] })));
  await screen.findByText('NextCanvas');
  expect(screen.queryByText('PreviousCanvas')).toBeNull();
  routeSearch.value = {};
});
