/**
 * 结果管理独立页。
 *
 * 旧前端对应 `modules/result-manager/result-manager.view.tsx`：
 * - 跨项目展示节点产物（表 / 报告 / 规则 / 模型）。
 * - 支持按名称搜索、按类型筛选、按时间排序、分页。
 * - 支持下载非报告结果、查看详情（报告类结果可在详情中以 CSV 形式下载）。
 *
 * 新前端实现：
 * 1. 调用 `apiClient.listNodeResults` 拉取当前用户 ownerId 的结果列表。
 * 2. 列表按 `NodeAllResultsVO` 展示：结果名、节点、类型、来源项目、计算模式、创建时间、操作。
 * 3. 点击“详情”打开 Modal，调用 `apiClient.getNodeResultDetail` 展示报告/表元数据与输出摘要。
 * 4. 点击“下载”调用 `apiClient.downloadData` 触发浏览器下载。
 * 5. 状态列显示 `pullFromTeeStatus`：RUNNING / SUCCESS / FAILED / 空。
 */
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Button, Modal, toast, Badge } from '@secretpad/design-system';
import type { NodeAllResultsVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { useAuthStore } from '../../features/auth/model/auth-store';

const RESULT_KINDS = [
  { key: 'table', label: '表' },
  { key: 'report', label: '报告' },
  { key: 'rule', label: '规则' },
  { key: 'model', label: '模型' },
];

const PAGE_SIZE = 10;

function formatTime(value?: string): string {
  if (!value) return '-';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
}

function kindBadge(kind?: string): 'success' | 'warning' | 'error' | 'default' {
  switch (kind) {
    case 'table':
      return 'default';
    case 'report':
      return 'success';
    case 'rule':
      return 'warning';
    case 'model':
      return 'error';
    default:
      return 'default';
  }
}

function resultName(result: NodeAllResultsVO): string {
  return result.nodeResultsVO?.productName || result.nodeResultsVO?.domainDataId || '-';
}

function resultNodeName(result: NodeAllResultsVO): string {
  return result.nodeName || result.nodeId || '-';
}

function resultKind(result: NodeAllResultsVO): string {
  return result.nodeResultsVO?.datatableType || 'table';
}

function resultStatus(result: NodeAllResultsVO): string {
  return result.nodeResultsVO?.pullFromTeeStatus || 'SUCCESS';
}

export const ResultsPage: React.FC = () => {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const ownerId = user?.ownerId || '';

  const [pageNumber, setPageNumber] = useState(1);
  const [search, setSearch] = useState('');
  const [kindFilter, setKindFilter] = useState<string[]>([]);
  const [sortRule, setSortRule] = useState<'asc' | 'desc'>('desc');
  const [detailResult, setDetailResult] = useState<NodeAllResultsVO | null>(null);
  const [isDetailOpen, setIsDetailOpen] = useState(false);

  const resultsQuery = useQuery({
    queryKey: ['node-results', ownerId, pageNumber, search, kindFilter, sortRule],
    queryFn: () =>
      apiClient.listNodeResults({
        ownerId,
        pageNumber,
        pageSize: PAGE_SIZE,
        nameFilter: search,
        kindFilters: kindFilter,
        timeSortingRule: sortRule,
      }),
    enabled: !!ownerId,
  });
  const results = resultsQuery.data?.nodeAllResultsVOList ?? [];
  const total = resultsQuery.data?.totalNodeResultNums ?? 0;

  const detailQuery = useQuery({
    queryKey: ['node-result-detail', detailResult?.nodeId, detailResult?.nodeResultsVO?.domainDataId],
    queryFn: () =>
      apiClient.getNodeResultDetail({
        nodeId: detailResult!.nodeId!,
        domainDataId: detailResult!.nodeResultsVO!.domainDataId!,
      }),
    enabled: !!detailResult?.nodeId && !!detailResult?.nodeResultsVO?.domainDataId,
  });

  const handleSearch = (value: string) => {
    setSearch(value);
    setPageNumber(1);
  };

  const toggleKind = (kind: string) => {
    setKindFilter((prev) => (prev.includes(kind) ? prev.filter((k) => k !== kind) : [...prev, kind]));
    setPageNumber(1);
  };

  const handleDownload = async (result: NodeAllResultsVO) => {
    if (!result.nodeId || !result.nodeResultsVO?.domainDataId) return;
    try {
      const blob = await apiClient.downloadData({
        nodeId: result.nodeId,
        domainDataId: result.nodeResultsVO.domainDataId,
      });
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${result.nodeResultsVO.domainDataId}`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
      toast.success(t('results.downloadSuccess'));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  const openDetail = (result: NodeAllResultsVO) => {
    setDetailResult(result);
    setIsDetailOpen(true);
  };

  const closeDetail = () => {
    setIsDetailOpen(false);
    setDetailResult(null);
  };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="space-y-4">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('results.title')}</h2>
          <p className="text-xs text-gray-500">{t('results.subtitle')}</p>
        </div>
      </div>

      <Card>
        <div className="flex flex-col md:flex-row gap-3 mb-4">
          <input
            type="text"
            value={search}
            onChange={(e) => handleSearch(e.target.value)}
            placeholder={t('results.searchPlaceholder')}
            className="px-3 py-2 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 focus:outline-none focus:border-blue-500 flex-1"
          />
          <select
            value={sortRule}
            onChange={(e) => setSortRule(e.target.value as 'asc' | 'desc')}
            className="px-3 py-2 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 focus:outline-none focus:border-blue-500"
          >
            <option value="desc">{t('results.sortDesc')}</option>
            <option value="asc">{t('results.sortAsc')}</option>
          </select>
        </div>

        <div className="flex flex-wrap gap-2 mb-4">
          {RESULT_KINDS.map((kind) => {
            const active = kindFilter.includes(kind.key);
            return (
              <button
                key={kind.key}
                type="button"
                onClick={() => toggleKind(kind.key)}
                className={`px-2 py-1 rounded-md text-xs border transition-colors ${
                  active
                    ? 'bg-blue-50 dark:bg-blue-900/20 border-blue-500 text-blue-700 dark:text-blue-300'
                    : 'bg-gray-50 dark:bg-gray-800 border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400'
                }`}
              >
                {t(`results.kind.${kind.key}`)}
              </button>
            );
          })}
        </div>

        {resultsQuery.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

        {resultsQuery.error && (
          <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
            {t('common.error', { message: String(resultsQuery.error.message) })}
          </div>
        )}

        <div className="overflow-x-auto">
          <table className="w-full text-xs border-collapse">
            <thead>
              <tr className="border-b border-gray-200 dark:border-gray-700 text-left text-gray-500">
                <th className="py-2 px-3 font-semibold">{t('results.name')}</th>
                <th className="py-2 px-3 font-semibold">{t('results.node')}</th>
                <th className="py-2 px-3 font-semibold">{t('results.kind')}</th>
                <th className="py-2 px-3 font-semibold">{t('results.project')}</th>
                <th className="py-2 px-3 font-semibold">{t('results.mode')}</th>
                <th className="py-2 px-3 font-semibold">{t('results.status')}</th>
                <th className="py-2 px-3 font-semibold">{t('results.createTime')}</th>
                <th className="py-2 px-3 font-semibold text-right">{t('common.action')}</th>
              </tr>
            </thead>
            <tbody>
              {results.length === 0 && !resultsQuery.isLoading && (
                <tr>
                  <td colSpan={8} className="py-8 text-center text-gray-400">
                    {t('results.noData')}
                  </td>
                </tr>
              )}
              {results.map((item, idx) => {
                const vo = item.nodeResultsVO;
                const status = resultStatus(item);
                return (
                  <tr
                    key={`${item.nodeId}-${vo?.domainDataId ?? idx}`}
                    className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50"
                  >
                    <td className="py-2 px-3 font-medium">{resultName(item)}</td>
                    <td className="py-2 px-3">{resultNodeName(item)}</td>
                    <td className="py-2 px-3">
                      <Badge status={kindBadge(resultKind(item))}>{t(`results.kind.${resultKind(item)}`)}</Badge>
                    </td>
                    <td className="py-2 px-3">{vo?.sourceProjectName || vo?.sourceProjectId || '-'}</td>
                    <td className="py-2 px-3">{vo?.computeMode || '-'}</td>
                    <td className="py-2 px-3">
                      <Badge status={status === 'SUCCESS' ? 'success' : status === 'FAILED' ? 'error' : 'default'}>{status}</Badge>
                    </td>
                    <td className="py-2 px-3">{formatTime(vo?.gmtCreate)}</td>
                    <td className="py-2 px-3 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <Button variant="ghost" size="sm" onClick={() => openDetail(item)}>
                          {t('common.detail')}
                        </Button>
                        <Button variant="outline" size="sm" onClick={() => handleDownload(item)} disabled={!item.nodeId || !vo?.domainDataId}>
                          {t('common.download')}
                        </Button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        {totalPages > 1 && (
          <div className="flex items-center justify-between mt-4 text-xs">
            <div className="text-gray-500">
              {t('results.total', { total, start: (pageNumber - 1) * PAGE_SIZE + 1, end: Math.min(pageNumber * PAGE_SIZE, total) })}
            </div>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={() => setPageNumber((p) => Math.max(1, p - 1))} disabled={pageNumber <= 1}>
                {t('common.previous')}
              </Button>
              <span className="text-gray-500">
                {pageNumber} / {totalPages}
              </span>
              <Button variant="ghost" size="sm" onClick={() => setPageNumber((p) => Math.min(totalPages, p + 1))} disabled={pageNumber >= totalPages}>
                {t('common.next')}
              </Button>
            </div>
          </div>
        )}
      </Card>

      <Modal
        isOpen={isDetailOpen}
        onClose={closeDetail}
        title={t('results.detailTitle')}
        width="max-w-2xl"
        footer={
          <Button variant="ghost" onClick={closeDetail}>
            {t('common.close')}
          </Button>
        }
      >
        <div className="text-xs space-y-3">
          {detailQuery.isLoading && <div>{t('common.loading')}</div>}
          {detailQuery.error && (
            <div className="text-red-500">{t('common.error', { message: String(detailQuery.error.message) })}</div>
          )}
          {detailQuery.data && (
            <>
              <div className="grid grid-cols-2 gap-2">
                <div className="text-gray-500">{t('results.name')}</div>
                <div>{detailQuery.data.nodeResultsVO?.productName || detailQuery.data.nodeResultsVO?.domainDataId || '-'}</div>
                <div className="text-gray-500">{t('results.node')}</div>
                <div>{detailResult?.nodeName || detailResult?.nodeId || '-'}</div>
                <div className="text-gray-500">{t('results.kind')}</div>
                <div>{t(`results.kind.${detailQuery.data.nodeResultsVO?.datatableType || 'table'}`)}</div>
                <div className="text-gray-500">{t('results.project')}</div>
                <div>{detailQuery.data.nodeResultsVO?.sourceProjectName || detailQuery.data.nodeResultsVO?.sourceProjectId || '-'}</div>
                <div className="text-gray-500">{t('results.mode')}</div>
                <div>{detailQuery.data.nodeResultsVO?.computeMode || '-'}</div>
                <div className="text-gray-500">{t('results.status')}</div>
                <div>{detailQuery.data.nodeResultsVO?.pullFromTeeStatus || '-'}</div>
                <div className="text-gray-500">{t('results.createTime')}</div>
                <div>{formatTime(detailQuery.data.nodeResultsVO?.gmtCreate)}</div>
                <div className="text-gray-500">{t('results.datasourceType')}</div>
                <div>{detailQuery.data.nodeResultsVO?.datasourceType || '-'}</div>
                <div className="text-gray-500">{t('results.relativeUri')}</div>
                <div className="break-all">{detailQuery.data.nodeResultsVO?.relativeUri || '-'}</div>
              </div>
              {detailQuery.data.output && (
                <div>
                  <div className="font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('results.output')}</div>
                  <pre className="bg-gray-50 dark:bg-gray-800 p-2 rounded-lg overflow-auto max-h-60 text-[10px] font-mono">
                    {JSON.stringify(detailQuery.data.output, null, 2)}
                  </pre>
                </div>
              )}
            </>
          )}
        </div>
      </Modal>
    </div>
  );
};

ResultsPage.displayName = 'ResultsPage';
