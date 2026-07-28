/**
 * 组件版本管理页面（Component Versions）。
 *
 * 对应旧前端“版本管理”模块与后端 `/api/v1alpha1/version/list` 接口：
 * 展示平台各核心组件（Kuscia、SecretFlow、Serving、DataProxy、SCQL、TEE 等）
 * 当前使用的 Docker 镜像版本，便于运维与交付人员快速确认部署版本。
 *
 * 设计要点：
 * 1. 调用 `apiClient.listComponentVersions()` 获取 `ComponentVersion` 对象；
 * 2. 将其各字段（xxxImage）映射为“组件名 + 镜像版本”卡片列表展示；
 * 3. 镜像版本用等宽字体展示，并自动从镜像 tag 中提取版本号高亮；
 * 4. 支持手动刷新。
 */
import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Button, Badge } from '@secretpad/design-system';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

/**
 * 组件字段 → 展示配置。
 *
 * key 为 `ComponentVersion` 的字段名，icon 为展示图标，labelKey 为 i18n 键。
 * 按“调度 → 计算 → 推理 → 控制台 → 数据 → 查询 → TEE”的顺序排列。
 */
const COMPONENT_FIELDS: Array<{ key: string; icon: string; labelKey: string }> = [
  { key: 'kusciaImage', icon: '⚙️', labelKey: 'versions.kusciaImage' },
  { key: 'secretflowImage', icon: '🔐', labelKey: 'versions.secretflowImage' },
  { key: 'secretflowServingImage', icon: '🚀', labelKey: 'versions.secretflowServingImage' },
  { key: 'secretpadImage', icon: '🖥️', labelKey: 'versions.secretpadImage' },
  { key: 'dataProxyImage', icon: '🔌', labelKey: 'versions.dataProxyImage' },
  { key: 'scqlImage', icon: '🗃️', labelKey: 'versions.scqlImage' },
  { key: 'teeAppImage', icon: '🛡️', labelKey: 'versions.teeAppImage' },
  { key: 'teeDmImage', icon: '🗄️', labelKey: 'versions.teeDmImage' },
  { key: 'capsuleManagerSimImage', icon: '💊', labelKey: 'versions.capsuleManagerSimImage' },
];

/**
 * 从镜像完整地址中提取版本 tag。
 *
 * 例如 `secretflow/kuscia:0.5.0` → `0.5.0`；若无 `:` 分隔则原样返回。
 * 用于在卡片上高亮展示版本号。
 */
function extractTag(image?: string): string {
  if (!image) return '-';
  // 镜像地址可能在 registry 中带端口（如 registry:5000/img:tag），取最后一个冒号后的部分。
  const idx = image.lastIndexOf(':');
  // 若最后一个冒号出现在路径分隔符之后，认为是 tag；否则视为无 tag。
  const slashIdx = image.lastIndexOf('/');
  if (idx > slashIdx && idx !== -1) return image.slice(idx + 1);
  return image;
}

export const ComponentVersionsPage: React.FC = () => {
  const { t } = useTranslation();

  // 组件版本查询。
  const versionsQuery = useQuery({
    queryKey: ['component-versions'],
    queryFn: () => apiClient.listComponentVersions(),
  });

  const versions = versionsQuery.data as Record<string, string | undefined> | undefined;
  const loading = versionsQuery.isLoading;
  const error = versionsQuery.error?.message || null;

  // 过滤出有值的组件（后端可能不返回全部字段）。
  const availableComponents = COMPONENT_FIELDS.filter((f) => versions?.[f.key]);

  return (
    <div className="space-y-6">
      {/* 页头 + 刷新 */}
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('versions.title')}</h2>
          <p className="text-xs text-gray-500">{t('versions.subtitle')}</p>
        </div>
        <Button size="sm" variant="outline" onClick={() => versionsQuery.refetch()} loading={loading}>
          {t('versions.refresh')}
        </Button>
      </div>

      {/* 错误提示 */}
      {error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error })}
        </div>
      )}

      {/* 加载中 */}
      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      {/* 组件版本卡片网格 */}
      {!loading && !error && (
        <>
          {availableComponents.length === 0 && (
            <div className="text-center text-xs text-gray-400 py-16">{t('versions.noData')}</div>
          )}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {availableComponents.map((field) => {
              const image = versions?.[field.key];
              const tag = extractTag(image);
              return (
                <Card key={field.key} bodyClassName="p-4">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <span className="w-9 h-9 rounded-lg bg-blue-50 dark:bg-blue-950/50 flex items-center justify-center text-lg">
                        {field.icon}
                      </span>
                      <div>
                        <div className="font-semibold text-sm text-gray-900 dark:text-gray-100">
                          {t(field.labelKey)}
                        </div>
                        <div className="text-[10px] text-gray-400 font-mono mt-0.5">{field.key}</div>
                      </div>
                    </div>
                    <Badge status="processing">
                      <span className="font-mono text-[10px]">{tag}</span>
                    </Badge>
                  </div>
                  {/* 完整镜像地址 */}
                  <div className="mt-3 p-2 rounded bg-gray-900 border border-gray-800 font-mono text-[10px] text-gray-400 break-all">
                    {image}
                  </div>
                </Card>
              );
            })}
          </div>
        </>
      )}
    </div>
  );
};
