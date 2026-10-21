import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { DAGNextWorkspace } from './index';
import type { DAGComponentDef, DAGNode } from './index';

/**
 * dag-next 画布拖拽 / 节点交互测试。
 *
 * 背景（回归来源）：/dag 页面曾出现「左侧组件无法拖到画布」的线上问题，
 * 根因有两层：
 * 1. 宿主页面在 readOnly 回退中渲染了画布（draggable=false）——由宿主权限修复；
 * 2. 宿主页面每次渲染都传入新的 initialNodes 引用，触发本组件
 *    useEffect([initialNodes]) 的全量同步，把刚拖入的节点立刻冲掉。
 * 本文件把画布的拖拽契约（可拖、可落、落点坐标、同步语义、readOnly 禁用）
 * 全部用测试固化，防止回退。
 */

/** jsdom 没有 DataTransfer 实现，这里造一个最小可用的存储型 mock。 */
function createDataTransfer() {
  const store: Record<string, string> = {};
  return {
    dropEffect: 'none',
    effectAllowed: 'all',
    types: [] as string[],
    files: [] as any,
    items: [] as any,
    setData: vi.fn((type: string, val: string) => {
      store[type] = val;
    }),
    getData: vi.fn((type: string) => store[type] ?? ''),
    clearData: vi.fn(() => {
      Object.keys(store).forEach((k) => delete store[k]);
    }),
    __store: store,
  };
}

const trainComponent: DAGComponentDef = {
  domain: 'ml.train',
  name: 'ss_sgd_train',
  version: '1.0.0',
  desc: 'Secure SGD train',
  icon: '🤖',
};

const componentGroups = { 'ML Train': [trainComponent] };

function renderWorkspace(props: Partial<Parameters<typeof DAGNextWorkspace>[0]> = {}) {
  return render(<DAGNextWorkspace componentGroups={componentGroups} {...props} />);
}

/** 中间画布节点：内部 svg 的父元素（带网格点背景的画布 div）。 */
function getCanvas(container: HTMLElement): HTMLElement {
  const svg = container.querySelector('svg.pointer-events-none');
  if (!svg || !svg.parentElement) throw new Error('canvas not found');
  return svg.parentElement as HTMLElement;
}

/** 让画布拥有一份非零的边界矩形，便于验证落点坐标换算。 */
function mockCanvasRect(canvas: HTMLElement) {
  canvas.getBoundingClientRect = () =>
    ({
      left: 0,
      top: 0,
      width: 1000,
      height: 800,
      right: 1000,
      bottom: 800,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    }) as DOMRect;
}

/** 从左侧组件库发起 HTML5 拖拽（dragStart 阶段写入 dataTransfer）。 */
function startDrag(item: HTMLElement, dataTransfer = createDataTransfer()) {
  fireEvent.dragStart(item, { dataTransfer });
  return dataTransfer;
}

/** 在画布上完成一次拖放（MouseEvent 构造以携带 clientX/clientY）。 */
function dropOnCanvas(
  canvas: HTMLElement,
  dataTransfer: ReturnType<typeof createDataTransfer>,
  clientX = 300,
  clientY = 200,
) {
  fireEvent.dragOver(canvas, { dataTransfer });
  const dropEvent = new MouseEvent('drop', {
    bubbles: true,
    cancelable: true,
    clientX,
    clientY,
  });
  Object.defineProperty(dropEvent, 'dataTransfer', { value: dataTransfer });
  fireEvent(canvas, dropEvent);
}

function getPaletteItem(name = trainComponent.name): HTMLElement {
  const el = screen.getByText(name).closest('[draggable]');
  if (!el) throw new Error(`palette item ${name} not found`);
  return el as HTMLElement;
}

/** 顶栏的节点/边计数文本，如 "1 nodes · 0 edges"。 */
function expectCounter(text: string) {
  expect(screen.getByText(new RegExp(`^${text}$`))).toBeTruthy();
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('组件库拖拽（drag & drop）', () => {
  it('编辑模式下组件条目可拖拽（draggable=true）', () => {
    renderWorkspace();
    expect(getPaletteItem().getAttribute('draggable')).toBe('true');
  });

  it('dragStart 把组件定义写入 dataTransfer（application/json + text/plain）', () => {
    renderWorkspace();
    const dt = startDrag(getPaletteItem());
    expect(dt.setData).toHaveBeenCalledWith('application/json', JSON.stringify(trainComponent));
    expect(dt.setData).toHaveBeenCalledWith('text/plain', 'ml.train/ss_sgd_train');
    expect(JSON.parse(dt.__store['application/json'])).toEqual(trainComponent);
  });

  it('dragOver 画布时标记 dropEffect=copy（允许落下）', () => {
    const { container } = renderWorkspace();
    const canvas = getCanvas(container);
    const dt = createDataTransfer();
    fireEvent.dragOver(canvas, { dataTransfer: dt });
    expect(dt.dropEffect).toBe('copy');
  });

  it('拖放组件到画布：调用 onAddNode 并把节点放到落点坐标', async () => {
    const onAddNode = vi.fn(async (component: DAGComponentDef): Promise<DAGNode> => ({
      id: 'node-from-host',
      name: component.name,
      category: component.domain,
      icon: component.icon || '⚙️',
      status: 'Ready',
      // 宿主返回的默认位置应被落点覆盖
      x: 0,
      y: 0,
      codeName: `${component.domain}/${component.name}`,
    }));
    const { container } = renderWorkspace({ onAddNode });
    const canvas = getCanvas(container);
    mockCanvasRect(canvas);

    const dt = startDrag(getPaletteItem());
    dropOnCanvas(canvas, dt, 300, 200);

    await waitFor(() => expectCounter('1 nodes · 0 edges'));
    expect(onAddNode).toHaveBeenCalledTimes(1);
    expect(onAddNode).toHaveBeenCalledWith(trainComponent);

    // 落点换算：x = 300 - rect.left - 70 = 230，y = 200 - rect.top - 20 = 180
    const nodeEl = within(canvas).getByText('ss_sgd_train').closest('.absolute') as HTMLElement;
    expect(nodeEl.style.left).toBe('230px');
    expect(nodeEl.style.top).toBe('180px');
  });

  it('无 onAddNode 时在本地构造节点（codeName = domain/name），并自动选中', async () => {
    const onNodeSelect = vi.fn();
    const { container } = renderWorkspace({ onNodeSelect });
    const canvas = getCanvas(container);
    mockCanvasRect(canvas);

    const dt = startDrag(getPaletteItem());
    dropOnCanvas(canvas, dt);

    await waitFor(() => expectCounter('1 nodes · 0 edges'));
    const nodeEl = within(canvas).getByText('ss_sgd_train').closest('.absolute');
    expect(nodeEl).toBeTruthy();
    // 自动选中新节点，右侧检查器展示其 codeName
    expect(onNodeSelect).toHaveBeenCalledWith(
      expect.objectContaining({ codeName: 'ml.train/ss_sgd_train' }),
    );
    expect(screen.getByText('ml.train/ss_sgd_train')).toBeTruthy();
  });

  it('点击组件条目同样能把组件加到画布', async () => {
    const { container } = renderWorkspace();
    const canvas = getCanvas(container);
    fireEvent.click(screen.getByText('ss_sgd_train'));
    await waitFor(() => expectCounter('1 nodes · 0 edges'));
    expect(within(canvas).getByText('ss_sgd_train')).toBeTruthy();
  });

  it('非法拖放内容不会新增节点', async () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { container } = renderWorkspace();
    const canvas = getCanvas(container);

    // 1) 没有 application/json 数据
    dropOnCanvas(canvas, createDataTransfer());
    // 2) JSON 无法解析
    const badJson = createDataTransfer();
    badJson.__store['application/json'] = '{not-json';
    dropOnCanvas(canvas, badJson);
    // 3) JSON 合法但缺少 domain/name
    const incomplete = createDataTransfer();
    incomplete.__store['application/json'] = JSON.stringify({ foo: 'bar' });
    dropOnCanvas(canvas, incomplete);

    await new Promise((r) => setTimeout(r, 20));
    expectCounter('0 nodes · 0 edges');
    expect(consoleSpy).toHaveBeenCalledTimes(1); // 仅第 2 种情况会记录解析错误
  });
});

describe('画布状态同步契约（initialNodes 引用语义）', () => {
  it('initialNodes 引用保持稳定时，重渲染不会冲掉新拖入的节点', async () => {
    // 宿主页面必须 memoize initialNodes（见 pages/dag/index.tsx 的 useMemo），
    // 本用例固化「引用稳定 => 本地新增节点存活」这一契约。
    const stableInitialNodes: DAGNode[] = [];
    const { container, rerender } = render(
      <DAGNextWorkspace componentGroups={componentGroups} initialNodes={stableInitialNodes} />,
    );
    const canvas = getCanvas(container);

    const dt = startDrag(getPaletteItem());
    dropOnCanvas(canvas, dt);
    await waitFor(() => expectCounter('1 nodes · 0 edges'));

    rerender(
      <DAGNextWorkspace componentGroups={componentGroups} initialNodes={stableInitialNodes} />,
    );
    expectCounter('1 nodes · 0 edges');
    expect(within(canvas).getByText('ss_sgd_train')).toBeTruthy();
  });

  it('initialNodes 引用变化时，画布会重新同步为传入内容', async () => {
    // 这是同步语义本身：引用变化意味着「服务端图数据已更新」，画布应跟随重置。
    // 宿主若未 memoize，每次重渲染都会误触发本行为 —— 这正是拖入节点瞬间消失的回归点。
    const { container, rerender } = render(
      <DAGNextWorkspace componentGroups={componentGroups} initialNodes={[]} />,
    );
    const canvas = getCanvas(container);

    const dt = startDrag(getPaletteItem());
    dropOnCanvas(canvas, dt);
    await waitFor(() => expectCounter('1 nodes · 0 edges'));

    rerender(<DAGNextWorkspace componentGroups={componentGroups} initialNodes={[]} />);
    expectCounter('0 nodes · 0 edges');
    expect(within(canvas).queryByText('ss_sgd_train')).toBeNull();
  });
});

describe('readOnly 只读模式', () => {
  it('组件条目不可拖拽，点击与拖放均不会新增节点', async () => {
    const { container } = renderWorkspace({ readOnly: true });
    const canvas = getCanvas(container);

    const item = getPaletteItem();
    expect(item.getAttribute('draggable')).toBe('false');

    // 点击添加被禁用
    fireEvent.click(screen.getByText('ss_sgd_train'));
    // 拖放被禁用
    const dt = startDrag(item);
    dropOnCanvas(canvas, dt);

    await new Promise((r) => setTimeout(r, 20));
    expectCounter('0 nodes · 0 edges');
    expect(within(canvas).queryByText('ss_sgd_train')).toBeNull();
  });
});

describe('画布节点交互', () => {
  const existingNodes: DAGNode[] = [
    { id: 'a', name: 'NodeA', category: 'ml', icon: '🤖', status: 'Ready', x: 100, y: 100 },
    { id: 'b', name: 'NodeB', category: 'ml', icon: '🤖', status: 'Ready', x: 400, y: 300 },
  ];

  it('拖动已有节点并松开后调用 onNodeMove（坐标随鼠标换算）', async () => {
    const onNodeMove = vi.fn();
    // 关闭网格吸附以验证纯坐标换算；吸附行为见下一用例。
    const { container } = renderWorkspace({ initialNodes: existingNodes, onNodeMove, defaultSnapToGrid: false });
    const canvas = getCanvas(container);
    mockCanvasRect(canvas);

    const nodeEl = within(canvas).getByText('NodeA').closest('.absolute') as HTMLElement;
    fireEvent.mouseDown(nodeEl, { clientX: 150, clientY: 150 });
    fireEvent.mouseMove(canvas, { clientX: 250, clientY: 280 });
    fireEvent.mouseUp(canvas);

    // offset = (150-100, 150-100) = (50, 50) → 新位置 = (250-50, 280-50) = (200, 230)
    await waitFor(() => expect(onNodeMove).toHaveBeenCalledTimes(1));
    expect(onNodeMove).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'a', x: 200, y: 230 }),
    );
  });

  it('网格吸附开启时 onNodeMove 收到的是吸附后的最终坐标（与画布显示一致）', async () => {
    const onNodeMove = vi.fn();
    const onGraphChange = vi.fn();
    const { container } = renderWorkspace({ initialNodes: existingNodes, onNodeMove, onGraphChange });
    const canvas = getCanvas(container);
    mockCanvasRect(canvas);

    const nodeEl = within(canvas).getByText('NodeA').closest('.absolute') as HTMLElement;
    fireEvent.mouseDown(nodeEl, { clientX: 150, clientY: 150 });
    fireEvent.mouseMove(canvas, { clientX: 250, clientY: 280 });
    fireEvent.mouseUp(canvas);

    // (200, 230) 吸附到 16px 网格 → (208, 224)
    await waitFor(() => expect(onNodeMove).toHaveBeenCalledWith(expect.objectContaining({ id: 'a', x: 208, y: 224 })));
    expect(nodeEl.style.left).toBe('208px');
    expect(onGraphChange).toHaveBeenCalledWith(expect.any(Array), expect.any(Array), 'move');
  });

  it('点击节点（未超过拖拽阈值）不会触发 onNodeMove', async () => {
    const onNodeMove = vi.fn();
    const { container } = renderWorkspace({ initialNodes: existingNodes, onNodeMove });
    const canvas = getCanvas(container);
    const nodeEl = within(canvas).getByText('NodeA').closest('.absolute') as HTMLElement;
    fireEvent.mouseDown(nodeEl, { clientX: 150, clientY: 150 });
    fireEvent.mouseMove(canvas, { clientX: 151, clientY: 151 });
    fireEvent.mouseUp(canvas);
    await new Promise((r) => setTimeout(r, 10));
    expect(onNodeMove).not.toHaveBeenCalled();
  });

  it('缩放后拖放：落点坐标按 zoom 换算为画布世界坐标', async () => {
    const onAddNode = vi.fn(async (component: DAGComponentDef): Promise<DAGNode> => ({
      id: 'n1', name: component.name, category: component.domain, icon: '⚙️', status: 'Ready', x: 0, y: 0,
    }));
    const { container } = renderWorkspace({ onAddNode });
    const canvas = getCanvas(container);
    mockCanvasRect(canvas);
    // 50% 以下封顶到 50：点击 5 次缩小到 50%
    const zoomOut = screen.getByRole('button', { name: '🔍 -' });
    for (let i = 0; i < 5; i++) fireEvent.click(zoomOut);
    expect(screen.getByText('50%')).toBeTruthy();
    const dt = startDrag(getPaletteItem());
    dropOnCanvas(canvas, dt, 300, 200);
    await waitFor(() => expectCounter('1 nodes · 0 edges'));
    const nodeEl = within(canvas).getByText('ss_sgd_train').closest('.absolute') as HTMLElement;
    // world = (300/0.5, 200/0.5) = (600, 400) → 左上角 (530, 380)
    expect(nodeEl.style.left).toBe('530px');
    expect(nodeEl.style.top).toBe('380px');
  });

  it('连线模式：依次点击源/目标节点后通过 onConnect 建立边', async () => {
    const onConnect = vi.fn((source: string, target: string) => ({
      id: 'edge-1',
      source,
      target,
    }));
    const { container } = renderWorkspace({ initialNodes: existingNodes, onConnect });
    const canvas = getCanvas(container);

    fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
    fireEvent.click(within(canvas).getByText('NodeA'));
    fireEvent.click(within(canvas).getByText('NodeB'));

    await waitFor(() => expectCounter('2 nodes · 1 edges'));
    expect(onConnect).toHaveBeenCalledWith('a', 'b');
  });

  it('删除节点按钮会移除节点', async () => {
    const { container } = renderWorkspace({ initialNodes: existingNodes });
    const canvas = getCanvas(container);
    expect(within(canvas).getByText('NodeA')).toBeTruthy();

    fireEvent.click(screen.getAllByTitle('Delete node')[0]);
    await waitFor(() => expectCounter('1 nodes · 0 edges'));
    expect(within(canvas).queryByText('NodeA')).toBeNull();
  });
});

describe('端口连线与撤销', () => {
  const portNodes: DAGNode[] = [
    {
      id: 'g-node-1', name: 'Reader', category: 'read_data', icon: '📥', status: 'Ready', x: 100, y: 100,
      ports: [{ id: 'g-node-1-output-0', group: 'output', index: 0, types: ['sf.table.individual'] }],
      outputs: ['g-node-1-output-0'],
    },
    {
      id: 'g-node-2', name: 'Psi', category: 'data_prep', icon: '⚙️', status: 'Ready', x: 400, y: 100,
      ports: [
        { id: 'g-node-2-input-0', group: 'input', index: 0, types: ['sf.table.individual'] },
        { id: 'g-node-2-input-1', group: 'input', index: 1, types: ['sf.table.individual'] },
        { id: 'g-node-2-output-0', group: 'output', index: 0, types: ['sf.table.vertical'] },
      ],
      outputs: ['g-node-2-output-0'],
    },
    {
      id: 'g-node-3', name: 'Model', category: 'ml.predict', icon: '⚙️', status: 'Ready', x: 700, y: 100,
      ports: [{ id: 'g-node-3-input-0', group: 'input', index: 0, types: ['sf.model.ss_glm'] }],
    },
  ];

  function portEl(container: HTMLElement, id: string) {
    const el = container.querySelector(`[data-port-id="${id}"]`);
    if (!el) throw new Error(`port ${id} not found`);
    return el as HTMLElement;
  }

  it('从输出端口拖到兼容的输入端口建立边，锚点为 nodeId-output-i / nodeId-input-i，并更新下游 inputs', async () => {
    const onGraphChange = vi.fn();
    const { container } = renderWorkspace({ initialNodes: portNodes, onGraphChange });
    fireEvent.mouseDown(portEl(container, 'g-node-1-output-0'));
    fireEvent.mouseUp(portEl(container, 'g-node-2-input-1'));
    await waitFor(() => expectCounter('3 nodes · 1 edges'));
    const [nodes, edges, reason] = onGraphChange.mock.calls.at(-1)!;
    expect(reason).toBe('connect');
    expect(edges[0]).toMatchObject({
      id: 'g-node-1-output-0__g-node-2-input-1',
      source: 'g-node-1',
      target: 'g-node-2',
      sourceAnchor: 'g-node-1-output-0',
      targetAnchor: 'g-node-2-input-1',
    });
    expect(nodes.find((n: DAGNode) => n.id === 'g-node-2').inputs).toEqual(['', 'g-node-1-output-0']);
  });

  it('类型不兼容的连线被拒绝并回调原因', async () => {
    const onConnectionRejected = vi.fn();
    const { container } = renderWorkspace({ initialNodes: portNodes, onConnectionRejected });
    fireEvent.mouseDown(portEl(container, 'g-node-1-output-0'));
    fireEvent.mouseUp(portEl(container, 'g-node-3-input-0'));
    await waitFor(() => expect(onConnectionRejected).toHaveBeenCalledWith('TYPE'));
    expectCounter('3 nodes · 0 edges');
  });

  it('连线与删除节点都进入撤销栈', async () => {
    const { container } = renderWorkspace({ initialNodes: portNodes });
    fireEvent.mouseDown(portEl(container, 'g-node-1-output-0'));
    fireEvent.mouseUp(portEl(container, 'g-node-2-input-0'));
    await waitFor(() => expectCounter('3 nodes · 1 edges'));
    fireEvent.click(screen.getAllByTitle('Delete node')[0]);
    await waitFor(() => expectCounter('2 nodes · 0 edges'));
    const undo = screen.getByTitle('Undo (Ctrl+Z)');
    fireEvent.click(undo);
    await waitFor(() => expectCounter('3 nodes · 1 edges'));
    fireEvent.click(undo);
    await waitFor(() => expectCounter('3 nodes · 0 edges'));
  });

  it('nodeStatuses 覆盖节点状态显示', () => {
    renderWorkspace({ initialNodes: portNodes, nodeStatuses: { 'g-node-2': { status: 'Running' } } });
    expect(screen.getByText('Running')).toBeTruthy();
  });

  it('未完成配置的节点显示“未配置”标记', () => {
    renderWorkspace({ initialNodes: [{ ...portNodes[0], configFinished: false }], labels: { unfinished: '未配置' } });
    expect(screen.getByText('未配置')).toBeTruthy();
  });

  it('只读模式下仍可打开日志 Tab 查看日志', async () => {
    const onNodeLogs = vi.fn(async () => ({ status: 'SUCCEED', logs: ['line-1'] }));
    const { container } = renderWorkspace({ initialNodes: portNodes, readOnly: true, onNodeLogs });
    const canvas = getCanvas(container);
    fireEvent.click(within(canvas).getByText('Reader'));
    fireEvent.click(screen.getByRole('button', { name: 'Logs' }));
    await waitFor(() => expect(onNodeLogs).toHaveBeenCalled());
    expect(await screen.findByText('line-1')).toBeTruthy();
  });
});

describe('centerOverlay（模板快速配置抽屉挂载点）', () => {
  it('渲染在画布行内（与画布同一定位容器），而不是覆盖全局 Header 的 fixed 浮层', async () => {
    const { TemplateQuickConfig } = await import('./template-quick-config');
    const { container } = renderWorkspace({
      centerOverlay: <TemplateQuickConfig templateType="PSI" labels={{ title: 'QC' }} />,
    });
    const dialog = screen.getByRole('dialog', { name: 'QC' });
    expect(dialog.className).toContain('absolute');
    expect(dialog.className).not.toContain('fixed');
    // 与画布共享同一个 relative 父容器（工具栏之下的画布行）。
    const canvas = container.querySelector('[data-tour="dag-canvas"]');
    expect(dialog.parentElement).toBe(canvas?.parentElement);
    expect(dialog.parentElement?.className).toContain('relative');
  });
});

describe('导入 JSON', () => {
  afterEach(() => vi.restoreAllMocks());

  it('按 allocateNodeIds 改写导入节点与连线 ID，并以 import 变更通知宿主保存', async () => {
    const json = JSON.stringify({
      nodes: [
        { id: 'x-node-1', name: 'A', category: 'c', icon: '', status: 'Succeeded', x: 0, y: 0, outputs: ['x-node-1-output-0'], ports: [{ id: 'x-node-1-output-0', kind: 'output', index: 0, types: [] }] },
        { id: 'x-node-2', name: 'B', category: 'c', icon: '', status: 'Failed', x: 0, y: 100, outputs: [], ports: [{ id: 'x-node-2-input-0', kind: 'input', index: 0, types: [] }] },
      ],
      edges: [{ id: 'e', source: 'x-node-1', target: 'x-node-2', sourceAnchor: 'x-node-1-output-0', targetAnchor: 'x-node-2-input-0' }],
    });
    let input: HTMLInputElement | null = null;
    const create = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation(((tag: string) => {
      const el = create(tag);
      if (tag === 'input') {
        input = el as HTMLInputElement;
        vi.spyOn(input, 'click').mockImplementation(() => undefined);
      }
      return el;
    }) as typeof document.createElement);
    const allocateNodeIds = vi.fn(async (n: number) => Array.from({ length: n }, (_, i) => `g1-node-${40 + i}`));
    const onGraphChange = vi.fn();
    renderWorkspace({ allocateNodeIds, onGraphChange });
    fireEvent.click(screen.getByTitle('Import JSON'));
    expect(input).toBeTruthy();
    const file = new File([json], 'g.json', { type: 'application/json' });
    Object.defineProperty(input!, 'files', { value: [file] });
    fireEvent.change(input!);
    await waitFor(() => expect(onGraphChange).toHaveBeenCalled());
    const [nodes, edges, reason] = onGraphChange.mock.calls.at(-1)!;
    expect(reason).toBe('import');
    expect(allocateNodeIds).toHaveBeenCalledWith(2);
    expect(nodes.map((n: DAGNode) => [n.id, n.status])).toEqual([
      ['g1-node-40', 'Ready'],
      ['g1-node-41', 'Ready'],
    ]);
    expect(nodes[1].inputs).toEqual(['g1-node-40-output-0']);
    expect(edges[0]).toMatchObject({ source: 'g1-node-40', target: 'g1-node-41', targetAnchor: 'g1-node-41-input-0' });
  });
});
