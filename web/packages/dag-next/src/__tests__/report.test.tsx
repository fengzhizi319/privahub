import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
import { NodeResultView } from '../result/result-view';
import { ResultVisualization } from '../result-visualization';
import {
  decodeValue,
  defaultPivotConfig,
  featureImportanceSeries,
  getTabName,
  parseReportTabs,
  pivotFlatTable,
  sampleSummary,
  sampleSummaryCsv,
  tabToCorrMatrix,
  tabToFlatTable,
  tabToPvaSeries,
  lexicographicalOrder,
} from '../result/report';
import type { FlatTable, ReportTab } from '../result/report';
import { normalizeOutput, resultKindOf } from '../result/output';

/** biclassification_eval 风格的 sf.report（Java graph/node/output：type=report, tabs=Tab[]）。 */
const biclsOutput = {
  type: 'report',
  codeName: 'ml.eval/biclassification_eval',
  gmtCreate: '2026-09-01T08:00:00Z',
  jobId: 'job1',
  taskId: 'job1-g-node-5',
  warning: ['label column has nulls'],
  tabs: [
    {
      name: 'SummaryReport',
      desc: 'Summary Report for bi-classification evaluation.',
      divs: [
        {
          children: [
            {
              type: 'descriptions',
              descriptions: {
                items: [
                  { name: 'total_samples', type: 'AT_INT', value: { i64: '1000' } },
                  { name: 'auc', type: 'AT_FLOAT', value: { f: 0.8321 } },
                  { name: 'ks', type: 'float', value: { f: 0.51 } },
                ],
              },
            },
          ],
        },
      ],
    },
    {
      name: 'eq_frequent_bin_report',
      divs: [
        {
          children: [
            {
              type: 'table',
              table: {
                headers: [
                  { name: 'start_value', type: 'AT_FLOAT' },
                  { name: 'positive', type: 'AT_INT' },
                  { name: 'label', type: 'AT_STRING' },
                ],
                rows: [
                  { name: 'bin_0', items: [{ f: 0.1 }, { i64: '12' }, { s: 'a' }] },
                  { name: 'bin_1', items: [{ f: 0.5 }, { i64: '30' }, { s: 'b' }] },
                ],
              },
            },
          ],
        },
      ],
    },
    {
      name: 'nested',
      divs: [
        {
          name: 'outer',
          children: [
            { type: 'div', div: { name: 'inner-div', children: [{ type: 'descriptions', descriptions: { name: 'meta', items: [{ name: 'deep_key', type: 'AT_BOOL', value: { b: true } }] } }] } },
            { type: 'table', table: { name: 'tbl', headers: [{ name: 'x', type: 'AT_STRINGS' }], rows: [{ name: 'r', items: [{ ss: ['p', 'q'] }] }] } },
          ],
        },
      ],
    },
  ],
};

const pearsonTab = {
  name: 'corr',
  divs: [
    {
      children: [
        {
          type: 'table',
          table: {
            headers: [
              { name: 'f1', type: 'AT_FLOAT' },
              { name: 'f2', type: 'AT_FLOAT' },
            ],
            rows: [
              { name: 'f1', items: [{ f: 1 }, { f: 0.3 }] },
              { name: 'f2', items: [{ f: 0.3 }, { f: 1 }] },
            ],
          },
        },
      ],
    },
  ],
};

describe('sf.report 解析', () => {
  it('Value 编码按声明类型解码（i64 字符串转数字、列表类型拼接）', () => {
    expect(decodeValue({ i64: '1000' }, 'AT_INT')).toBe(1000);
    expect(decodeValue({ f: 0.5 }, 'float')).toBe(0.5);
    expect(decodeValue({ s: 'x' }, 'str')).toBe('x');
    expect(decodeValue({ b: false }, 'AT_BOOL')).toBe(false);
    expect(decodeValue({ fs: [1, '2'] }, 'AT_FLOATS')).toEqual([1, 2]);
    // 类型与字段不一致时按存在字段兜底
    expect(decodeValue({ f: 3 }, 'AT_INT')).toBe(3);
    expect(decodeValue({ i64: '9007199254740993' }, 'AT_INT')).toBe('9007199254740993');
  });

  it('兼容字符串 / Report 对象 / DistData meta 形态的 tabs', () => {
    expect(parseReportTabs(JSON.stringify(biclsOutput.tabs))).toHaveLength(3);
    expect(parseReportTabs({ name: 'r', tabs: biclsOutput.tabs })).toHaveLength(3);
    expect(parseReportTabs({ meta: { tabs: biclsOutput.tabs } })).toHaveLength(3);
    expect(parseReportTabs(null)).toEqual([]);
  });

  it('descriptions → name/value 两列；table → 首列补 name（scql_analysis 不补）', () => {
    const d = tabToFlatTable(biclsOutput.tabs[0] as never);
    expect(d.kind).toBe('descriptions');
    expect(d.rows).toEqual([
      ['total_samples', 1000],
      ['auc', 0.8321],
      ['ks', 0.51],
    ]);
    const t = tabToFlatTable(biclsOutput.tabs[1] as never);
    expect(t.columns.map((c) => c.name)).toEqual(['name', 'start_value', 'positive', 'label']);
    expect(t.rows[0]).toEqual(['bin_0', 0.1, 12, 'a']);
    expect(tabToFlatTable(biclsOutput.tabs[1] as never, 'stats/scql_analysis').columns[0].name).toBe('start_value');
  });

  it('Tab 名映射与字典序', () => {
    expect(getTabName('SummaryReport', 0)).toBe('总评估表');
    expect(getTabName(undefined, 1)).toBe('输出表2');
    expect(['f10', 'f2', 'f1'].sort(lexicographicalOrder)).toEqual(['f1', 'f2', 'f10']);
  });

  it('相关系数矩阵 / PVA 转换', () => {
    expect(tabToCorrMatrix(pearsonTab as never)).toEqual({ labels: ['f1', 'f2'], matrix: [[1, 0.3], [0.3, 1]] });
    const pva = {
      divs: [
        {
          children: [
            {
              type: 'table',
              table: {
                headers: [
                  { name: 'interval', type: 'AT_STRING' },
                  { name: 'avg_prediction', type: 'AT_FLOAT' },
                  { name: 'avg_label', type: 'AT_FLOAT' },
                  { name: 'bias', type: 'AT_FLOAT' },
                ],
                rows: [{ name: '0', items: [{ s: '[0,0.5)' }, { f: 0.2 }, { f: 0.25 }, { f: 0.05 }] }],
              },
            },
          ],
        },
      ],
    };
    expect(tabToPvaSeries(pva as never)).toEqual([{ label: '[0,0.5)', avg_prediction: 0.2, avg_label: 0.25, bias: 0.05 }]);
  });
});

describe('输出规范化与分派', () => {
  it('兼容 DistData 类型名与旧 Go 形态', () => {
    expect(resultKindOf('sf.table.vertical')).toBe('table');
    expect(resultKindOf('sf.model.ss_glm')).toBe('model');
    expect(resultKindOf('sf.rule.binning')).toBe('rule');
    expect(resultKindOf('sf.report')).toBe('report');
    expect(resultKindOf('sf.serving.model')).toBe('serving');
    const go = normalizeOutput({ graph_node_id: 'n', type: 'table', meta: { rows: [{ tableId: 't', domainDataId: 'd' }], columns: [{ name: 'a', type: 'int' }] } })!;
    expect(go.rows[0]).toMatchObject({ tableId: 't', domainDataId: 'd', fields: ['a'], fieldTypes: ['int'] });
  });

  it('报告：生成时间、警告折叠、Tab 切换、嵌套 div 渲染', () => {
    render(<NodeResultView output={biclsOutput} outputId="g-node-5-output-0" />);
    expect(screen.getByText('报告')).toBeTruthy();
    expect(screen.getByText(/生成时间/)).toBeTruthy();
    expect(screen.getByText('Warning (1)')).toBeTruthy();
    expect(screen.getByText('auc')).toBeTruthy();
    expect(screen.getByText('0.8321')).toBeTruthy();
    fireEvent.click(screen.getByRole('tab', { name: '等频' }));
    expect(screen.getByText('bin_1')).toBeTruthy();
    fireEvent.click(screen.getByRole('tab', { name: 'nested' }));
    expect(screen.getByText('inner-div')).toBeTruthy();
    expect(screen.getByText('deep_key')).toBeTruthy();
    expect(screen.getByText('true')).toBeTruthy();
    expect(screen.getByText('p, q')).toBeTruthy();
  });

  it('codeName 可视化：ss_pearsonr 默认表格视图，可切换色块矩阵、全选 / 取消特征并全屏', () => {
    const { container } = render(<NodeResultView output={{ type: 'report', codeName: 'stats/ss_pearsonr', tabs: [pearsonTab] }} />);
    // 旧版 CorrMatrix 默认「表格」Tab。
    expect(container.querySelector('svg[aria-label="correlation heatmap"]')).toBeFalsy();
    fireEvent.click(screen.getByRole('button', { name: '色块矩阵' }));
    expect(container.querySelector('svg[aria-label="correlation heatmap"]')).toBeTruthy();
    const cellsAll = container.querySelectorAll('svg[aria-label="correlation heatmap"] rect title').length;
    fireEvent.click(screen.getByRole('button', { name: '全部' }));
    expect(container.querySelectorAll('svg[aria-label="correlation heatmap"] rect title').length).toBeLessThan(cellsAll);
    // 外层是整份报告的全屏，最后一个是色块矩阵图表自身的全屏。
    const fullBtns = screen.getAllByRole('button', { name: /全屏/ });
    fireEvent.click(fullBtns[fullBtns.length - 1]);
    expect(screen.getByRole('button', { name: /退出全屏/ })).toBeTruthy();
  });

  it('ss_pearsonr 色块坐标轴：方块按 |r| 缩放、对角线不画、正负分色', () => {
    const { container } = render(<NodeResultView output={{ type: 'report', codeName: 'stats/ss_pearsonr', tabs: [pearsonTab] }} />);
    fireEvent.click(screen.getByRole('button', { name: '色块坐标轴' }));
    const svg = container.querySelector('svg[aria-label="correlation axis chart"]');
    expect(svg).toBeTruthy();
    const rects = Array.from(svg!.querySelectorAll('rect'));
    expect(rects.length).toBeGreaterThan(0);
    // 对角线（自相关 = 1）不绘制。
    rects.forEach((r) => {
      const [a, b] = (r.querySelector('title')?.textContent || '').split(':')[0].split(' × ');
      expect(a).not.toBe(b);
    });
  });

  it('表结果：按参与方分行展示 schema，支持节点筛选与 TEE 申请', () => {
    let applied = '';
    render(
      <NodeResultView
        output={{
          type: 'table',
          codeName: 'data_prep/psi',
          meta: {
            headers: [],
            rows: [
              { nodeId: 'alice', path: 'out/alice.csv', tableId: 't1', fields: 'id,age', fieldTypes: 'str,int', type: 'embedded' },
              { nodeId: 'bob', path: 'out/bob.csv', tableId: 't1', fields: 'id,income', fieldTypes: 'str,float' },
            ],
          },
        }}
        outputId="g-node-2-output-0"
        downloadMode="tee"
        actions={{ onApplyTeeDownload: (row) => (applied = row.nodeId ?? '') }}
      />,
    );
    expect(screen.getByText('income')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('节点'), { target: { value: 'alice' } });
    expect(screen.queryByText('income')).toBeNull();
    fireEvent.click(screen.getAllByText('申请下载')[1]);
    expect(applied).toBe('bob');
  });

  it('模型结果：路径展示；read_model 产出不可下载', () => {
    render(
      <NodeResultView
        output={{ type: 'model', codeName: 'ml.predict/read_model', meta: { rows: [{ nodeId: 'alice', path: 'model/path' }] } }}
        downloadMode="direct"
        actions={{ onDownload: () => undefined }}
      />,
    );
    expect(screen.getByText('model/path')).toBeTruthy();
    expect(screen.queryByText('下载')).toBeNull();
  });

  it('无 type：提示非数据参与方；ResultVisualization 按 DistData 类型委托', () => {
    render(<NodeResultView output={{ codeName: 'stats/scql_analysis', jobId: 'j' }} outputId="o" />);
    expect(screen.getByText('非数据参与方，无计算结果')).toBeTruthy();
    const { container } = render(<ResultVisualization output={biclsOutput} />);
    expect(within(container).getByText('auc')).toBeTruthy();
  });
});

const descTab = (name: string, items: Array<[string, number]>): ReportTab => ({
  name,
  divs: [{ children: [{ type: 'descriptions', descriptions: { items: items.map(([n, v]) => ({ name: n, type: 'AT_FLOAT', value: { f: v } })) } }] }],
});

const groupbyFlat: FlatTable = {
  kind: 'table',
  columns: [
    { name: 'name', type: 'str' },
    { name: 'city', type: 'str' },
    { name: 'sex', type: 'str' },
    { name: 'age_mean', type: 'float' },
    { name: 'cnt', type: 'int' },
  ],
  rows: [
    ['0', 'hz', 'm', 30, 2],
    ['1', 'hz', 'f', 28, 3],
    ['2', 'sh', 'm', 40, 1],
  ],
};

describe('结果视图升级：全屏、透视拖拽、特征重要性、采样', () => {
  it('整份报告与表结果字段表都可全屏', () => {
    const { unmount } = render(<NodeResultView output={biclsOutput} />);
    fireEvent.click(screen.getAllByRole('button', { name: /全屏/ })[0]);
    expect(screen.getByRole('button', { name: /退出全屏/ })).toBeTruthy();
    unmount();
    render(
      <NodeResultView
        output={{ type: 'table', meta: { rows: [{ nodeId: 'alice', path: 'p', fields: 'id,age', fieldTypes: 'str,int' }] } }}
        outputId="o"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /全屏/ }));
    expect(screen.getByRole('button', { name: /退出全屏/ })).toBeTruthy();
    expect(screen.getByText('age')).toBeTruthy();
  });

  it('pivotFlatTable：默认行 / 列 / 值，支持多值与无列维度', () => {
    const cfg = defaultPivotConfig(groupbyFlat);
    expect(cfg).toEqual({ rows: [1], columns: [2], values: [3] });
    const p = pivotFlatTable(groupbyFlat, cfg);
    expect(p.columns.map((c) => c.name)).toEqual(['city \\ sex', 'm', 'f']);
    expect(p.rows).toEqual([
      ['hz', 30, 28],
      ['sh', 40, '-'],
    ]);
    const multi = pivotFlatTable(groupbyFlat, { rows: [2], columns: [], values: [3, 4] });
    expect(multi.columns.map((c) => c.name)).toEqual(['sex', 'age_mean', 'cnt']);
    expect(multi.rows).toEqual([
      ['m', 70, 3],
      ['f', 28, 3],
    ]);
  });

  it('groupby 交叉透视：字段可切换到行 / 列 / 值区域并重算', () => {
    const tab = {
      name: 'g',
      divs: [
        {
          children: [
            {
              type: 'table',
              table: {
                headers: [
                  { name: 'city', type: 'AT_STRING' },
                  { name: 'sex', type: 'AT_STRING' },
                  { name: 'age_mean', type: 'AT_FLOAT' },
                ],
                rows: [
                  { name: '0', items: [{ s: 'hz' }, { s: 'm' }, { f: 30 }] },
                  { name: '1', items: [{ s: 'sh' }, { s: 'f' }, { f: 40 }] },
                ],
              },
            },
          ],
        },
      ],
    };
    render(<NodeResultView output={{ type: 'report', codeName: 'stats/groupby_statistics', tabs: [tab] }} />);
    fireEvent.click(screen.getByRole('button', { name: '交叉透视表' }));
    expect(screen.getByText('city \\ sex')).toBeTruthy();
    // 把 sex 从「列」移到「行」：表头只剩行维度组合。
    fireEvent.change(screen.getByLabelText('sex 待选字段'), { target: { value: 'rows' } });
    expect(screen.getByText('city / sex')).toBeTruthy();
    expect(screen.getByText('hz / m')).toBeTruthy();
    // 拖拽：age_mean 拖回「待选」→ 无值字段时退回原表。
    const zone = screen.getByTestId('pivot-zone-fields');
    const data = new Map<string, string>();
    const dataTransfer = { setData: (k: string, v: string) => data.set(k, v), getData: (k: string) => data.get(k) ?? '' };
    fireEvent.dragStart(screen.getByText('age_mean', { selector: 'span' }), { dataTransfer });
    fireEvent.drop(zone, { dataTransfer });
    expect(within(screen.getByTestId('pivot-zone-fields')).getByText('age_mean')).toBeTruthy();
    expect(screen.queryByText('city / sex')).toBeNull();
  });

  it('特征重要性：sgb_train 报告（descriptions）按权重降序绘制条形图', () => {
    const tab = descTab('gain', [
      ['f_a', 0.2],
      ['f_b', 0.7],
    ]);
    expect(featureImportanceSeries(tabToFlatTable(tab), { allowDescriptions: true })).toEqual([
      { label: 'f_b', value: 0.7 },
      { label: 'f_a', value: 0.2 },
    ]);
    expect(featureImportanceSeries(tabToFlatTable(tab))).toBeNull();
    render(<NodeResultView output={{ type: 'report', codeName: 'ml.train/sgb_train', tabs: [tab] }} />);
    expect(screen.getByText('特征重要性')).toBeTruthy();
    expect(document.querySelectorAll('svg rect').length).toBeGreaterThan(0);
  });

  it('数据集采样：汇总各分层的采样前后数量与采样率，CSV 表头与旧版一致', () => {
    const tabs = [
      descTab('stratum_0', [
        ['num_before_sample', 100],
        ['num_after_sample', 10],
        ['sample_rate', 0.1],
      ]),
      descTab('stratum_1', [
        ['num_before_sample', 50],
        ['num_after_sample', 25],
        ['sample_rate', 0.5],
      ]),
    ];
    const summary = sampleSummary(parseReportTabs(tabs));
    expect(summary).toEqual([
      { name: 'stratum_0', numBefore: 100, numAfter: 10, sampleRate: 0.1 },
      { name: 'stratum_1', numBefore: 50, numAfter: 25, sampleRate: 0.5 },
    ]);
    expect(sampleSummaryCsv(summary)[0]).toEqual(['sample_name', 'num_before_sample', 'num_after_sample', 'sample_rate']);
    render(<NodeResultView output={{ type: 'report', codeName: 'data_filter/sample', tabs }} />);
    expect(screen.getByText('采样汇总')).toBeTruthy();
    expect(screen.getAllByText('stratum_1').length).toBeGreaterThan(0);
  });
});
