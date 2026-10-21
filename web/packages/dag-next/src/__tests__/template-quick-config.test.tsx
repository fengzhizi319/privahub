import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { TemplateQuickConfig } from '../template-quick-config';

const tables = [
  { datatableId: 't1', datatableName: 'alice_t', nodeId: 'alice', nodeName: 'alice', isPartitionTable: true },
  { datatableId: 't2', datatableName: 'bob_t', nodeId: 'bob', nodeName: 'bob' },
];
const fetchColumns = async (t: { datatableId: string }) => (t.datatableId === 't1' ? ['id', 'age', 'y'] : ['id', 'income']);

describe('TemplateQuickConfig（旧版 template-quick-config 字段）', () => {
  it('PSI(MPC)：分区表显示分区输入，缺字段时给出旧版校验提示且不保存', async () => {
    const onSave = vi.fn();
    render(<TemplateQuickConfig templateType="PSI" tables={tables} fetchColumns={fetchColumns} onSave={onSave} />);
    expect(screen.getByText(/若无数据集/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText('dataTableReceiver'), { target: { value: 't1' } });
    expect(screen.getByLabelText('dataTableReceiverPartition')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('dataTableSender'), { target: { value: 't1' } });
    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    expect(await screen.findByText('不能选择同一份样本表')).toBeTruthy();
    expect(screen.getAllByText('至少选择1列作为关联键').length).toBeGreaterThan(0);
    expect(screen.getAllByText('请选择特征列').length).toBeGreaterThan(1);
    expect(onSave).not.toHaveBeenCalled();
  });

  it('K-匿名：默认选中 alice 表，QI 默认 age、SA 默认最后一列，保存为旧版值形状', async () => {
    const onSave = vi.fn();
    render(<TemplateQuickConfig templateType="K_ANONYMITY" tables={tables} fetchColumns={fetchColumns} onSave={onSave} />);
    await waitFor(() => expect(screen.getByText('age')).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    await waitFor(() => expect(onSave).toHaveBeenCalledWith({ dataTable: { s: 't1' }, qiCols: ['age'], saCols: ['y'] }));
  });
});
