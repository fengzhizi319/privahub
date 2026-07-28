/**
 * 隐私场景展示页。
 *
 * 旧前端 `privacy-scenes` 模块用于向用户展示隐私计算的核心应用场景，
 * 并支持一键跳转到对应能力（如创建项目、打开 DAG 模板）。
 *
 * 本页面以卡片式布局展示 SecretPad 支持的主要隐私计算场景，每个场景包含：
 * - 场景名称与简要说明
 * - 涉及的核心技术标签
 * - 快速入口按钮（跳转到 DAG 或相关页面）
 */
import React from 'react';
import { useNavigate } from '@tanstack/react-router';
import { Card, Button, Badge } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';

interface PrivacyScene {
  key: string;
  tags: string[];
  route: string;
}

const scenes: PrivacyScene[] = [
  { key: 'psi', tags: ['PSI', 'Privacy Set Intersection'], route: '/dag' },
  { key: 'mpcRisk', tags: ['MPC', 'LR', 'WOE'], route: '/dag' },
  { key: 'tee', tags: ['TEE', 'Trusted Execution'], route: '/dag' },
  { key: 'classification', tags: ['Data Classification', 'L1-L5'], route: '/data-tables' },
  { key: 'sanitization', tags: ['Masking', 'Data Sanitization'], route: '/dag' },
  { key: 'kAnonymity', tags: ['K-Anonymity', 'Anonymization'], route: '/dag' },
  { key: 'lDiversity', tags: ['L-Diversity', 'Anonymization'], route: '/dag' },
  { key: 'localDp', tags: ['Local DP', 'Differential Privacy'], route: '/dag' },
  { key: 'dpQuery', tags: ['DP Query', 'Differential Privacy'], route: '/dag' },
  { key: 'queryObfuscation', tags: ['Query Obfuscation', 'Privacy'], route: '/dag' },
  { key: 'federatedLearning', tags: ['FL', 'Federated Learning'], route: '/dag' },
];

export const PrivacyScenesPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  return (
    <div className="space-y-6">
      <div className="bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('privacyScenes.title')}</h2>
        <p className="text-xs text-gray-500 mt-1">{t('privacyScenes.subtitle')}</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {scenes.map((scene) => (
          <Card key={scene.key} className="flex flex-col justify-between h-full">
            <div>
              <h3 className="text-sm font-bold text-gray-900 dark:text-gray-100 mb-2">
                {t(`privacyScenes.scene.${scene.key}.title`)}
              </h3>
              <p className="text-xs text-gray-500 mb-3 leading-relaxed">
                {t(`privacyScenes.scene.${scene.key}.desc`)}
              </p>
              <div className="flex flex-wrap gap-1.5 mb-4">
                {scene.tags.map((tag) => (
                  <Badge key={tag} status="default" className="text-[10px]">
                    {tag}
                  </Badge>
                ))}
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => navigate({ to: scene.route })}
            >
              {t('privacyScenes.tryIt')}
            </Button>
          </Card>
        ))}
      </div>
    </div>
  );
};

PrivacyScenesPage.displayName = 'PrivacyScenesPage';
