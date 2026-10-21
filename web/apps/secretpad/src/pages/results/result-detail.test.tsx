import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import type { NodeResultDetailVO } from '@secretpad/api-client';

const triggerBlobDownload = vi.fn();
vi.mock('../../features/job-detail', () => ({ triggerBlobDownload: (...args: unknown[]) => triggerBlobDownload(...args) }));

import { I18nProvider } from '../../shared/lib/i18n';
import { ResultDetailView } from './result-detail';

const reportDetail = {
  nodeResultsVO: { domainDataId: 'rpt-1', productName: 'report-1', datatableType: 'report' },
  output: {
    codeName: 'stats/table_statistics',
    tabs: [
      {
        name: 'summary',
        divs: [{ children: [{ type: 'descriptions', descriptions: { items: [{ name: 'rows', type: 'AT_INT', value: { i64: '10' } }] } }] }],
      },
    ],
  },
} as unknown as NodeResultDetailVO;

describe('ResultDetailView', () => {
  it('downloads a report as CSV from the detail drawer (same export as the list)', async () => {
    render(
      <I18nProvider>
        <ResultDetailView detail={reportDetail} result={{ nodeId: 'alice' } as never} />
      </I18nProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: '下载报告' }));
    expect(triggerBlobDownload).toHaveBeenCalledTimes(1);
    const [blob, name] = triggerBlobDownload.mock.calls[0] as [Blob, string];
    expect(name).toBe('rpt-1.csv');
    expect(await blob.text()).toContain('rows,10');
    // 整份报告可全屏。
    expect(screen.getAllByRole('button', { name: /全屏/ }).length).toBeGreaterThan(0);
  });

  it('lets the table field list go fullscreen', () => {
    const detail = {
      nodeResultsVO: { domainDataId: 't1', datatableType: 'table' },
      tableColumnVOList: [{ colName: 'age', colType: 'int' }],
    } as unknown as NodeResultDetailVO;
    render(
      <I18nProvider>
        <ResultDetailView detail={detail} result={{ nodeId: 'alice' } as never} />
      </I18nProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: /全屏/ }));
    expect(screen.getByRole('button', { name: /退出全屏/ })).toBeTruthy();
    expect(screen.queryByRole('button', { name: '下载报告' })).toBeNull();
  });
});
