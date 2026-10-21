/**
 * 结果管理独立页。
 *
 * 旧前端对应 `modules/result-manager/result-manager.view.tsx`：
 * - 跨项目展示节点产物（表 / 报告 / 规则 / 模型）。
 * - 支持按名称搜索、按类型筛选、按时间排序、分页。
 * - 支持下载非报告结果、查看详情（报告类结果可在详情中以 CSV 形式下载）。
 *
 * 新前端实现：
 * 1. 列表部分见 `./result-list.tsx`（node/result/list 分页、筛选、节点选择、批量下载/删除、TEE 申请/重试）。
 * 2. 列表点击“详情”回调 `openDetail(NodeAllResultsVO)`。
 * 3. 点击“详情”打开 Modal，调用 `apiClient.getNodeResultDetail`，由 `./result-detail.tsx` 渲染
 *    描述、血缘 DAG 预览、报告（sf.report）、字段表（分类分级 L1~L5 标注）与 TEE 重试。
 * 4. 点击“下载”调用 `apiClient.downloadData` 触发浏览器下载。
 * 5. 状态列显示 `pullFromTeeStatus`：RUNNING / SUCCESS / FAILED / 空。
 */
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Button, Drawer } from '@secretpad/design-system';
import type { NodeAllResultsVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { ResultList } from './result-list';
import { ResultDetailView } from './result-detail';

export const ResultsPage: React.FC = () => {
  const { t } = useTranslation();
  const [detailResult, setDetailResult] = useState<NodeAllResultsVO | null>(null);
  const [isDetailOpen, setIsDetailOpen] = useState(false);

  const detailQuery = useQuery({
    queryKey: ['node-result-detail', detailResult?.nodeId, detailResult?.nodeResultsVO?.domainDataId],
    queryFn: () =>
      apiClient.getNodeResultDetail({
        nodeId: detailResult!.nodeId!,
        domainDataId: detailResult!.nodeResultsVO!.domainDataId!,
      }),
    enabled: !!detailResult?.nodeId && !!detailResult?.nodeResultsVO?.domainDataId,
  });

  const openDetail = (result: NodeAllResultsVO) => {
    setDetailResult(result);
    setIsDetailOpen(true);
  };

  const closeDetail = () => {
    setIsDetailOpen(false);
    setDetailResult(null);
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('results.title')}</h2>
          <p className="text-xs text-gray-500">{t('results.subtitle')}</p>
        </div>
      </div>

      <ResultList onOpenDetail={openDetail} />

      <Drawer
        isOpen={isDetailOpen}
        onClose={closeDetail}
        title={t('results.detailTitle')}
        width="max-w-4xl"
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
          {detailQuery.data && <ResultDetailView key={detailResult?.nodeResultsVO?.domainDataId} detail={detailQuery.data} result={detailResult} />}
        </div>
      </Drawer>
    </div>
  );
};

ResultsPage.displayName = 'ResultsPage';
