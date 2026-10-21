/**
 * 隐私场景展示页。
 *
 * 旧前端 `privacy-scenes` 模块用于向用户展示隐私计算的核心应用场景，
 * 并支持一键跳转到对应能力（如创建项目、打开 DAG 模板）。
 *
 * 本页面以卡片式布局展示 SecretPad 支持的主要隐私计算场景，每个场景包含：
 * - 场景名称与简要说明
 * - 涉及的核心技术标签
 * - 一键创建：打开创建项目向导，预选对应 DAG 模板与默认节点 alice/bob
 *   （对应旧前端 privacy-scenes 的 quick create / buildScenarioQuickConfigs）
 */
import React, { useState } from 'react';
import { Card, Button, Badge } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';
import { CreateProjectWizard } from '../../features/create-project';
import type { CreateProjectPreset } from '../../features/create-project';
import { projectPermissions } from '../projects/project-list.logic';
import { templateByKey, categoryNameKey, templateCategories } from '../../features/dag-templates/registry';
import { scenes, scenePreset } from './scenes';

export const PrivacyScenesPage: React.FC = () => {
  const { t } = useTranslation();
  const platform = usePlatform();
  const { canCreate } = projectPermissions(platform);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('all');
  const [preset, setPreset] = useState<CreateProjectPreset | null>(null);

  const visibleScenes = scenes.filter(scene => {
    const group = templateByKey(scene.templateKey)?.metadata.category || 'basic';
    return (category === 'all' || category === group) && [t(`privacyScenes.scene.${scene.key}.title`), t(`privacyScenes.scene.${scene.key}.desc`), ...scene.tags].join(' ').toLowerCase().includes(search.trim().toLowerCase());
  });
  return (
    <div className="space-y-6">
      <div className="bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('privacyScenes.title')}</h2>
        <p className="text-xs text-gray-500 mt-1">{t('privacyScenes.subtitle')}</p>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <input aria-label={t('projectBrowser.sceneSearch')} placeholder={t('projectBrowser.sceneSearch')} value={search} onChange={event => setSearch(event.target.value)} className="w-full rounded-lg border border-gray-200 bg-white p-2.5 text-sm dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 sm:w-80" />
        {['all', ...templateCategories.filter(value => scenes.some(scene => templateByKey(scene.templateKey)?.metadata.category === value))].map(value => <button key={value} type="button" aria-pressed={category === value} onClick={() => setCategory(value)} className={`rounded-full border px-3 py-2 text-xs ${category === value ? 'border-blue-300 bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300' : 'border-gray-200 text-gray-500 dark:border-gray-700'}`}>{value === 'all' ? t('projectBrowser.all') : t(categoryNameKey[value as keyof typeof categoryNameKey])}</button>)}
      </div>
      {!visibleScenes.length && <p className="py-10 text-center text-sm text-gray-500">{t('common.empty')}</p>}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {visibleScenes.map((scene) => {
          const scenePresetValue = scenePreset(scene, t(`privacyScenes.scene.${scene.key}.title`), platform);
          return (
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
              disabled={!canCreate || !scenePresetValue}
              title={!scenePresetValue ? t('privacyScenes.modeUnsupported') : undefined}
              onClick={() => scenePresetValue && setPreset(scenePresetValue)}
            >
              {t('privacyScenes.oneClickCreate')}
            </Button>
          </Card>
          );
        })}
      </div>

      <CreateProjectWizard isOpen={!!preset} onClose={() => setPreset(null)} preset={preset ?? undefined} />
    </div>
  );
};

PrivacyScenesPage.displayName = 'PrivacyScenesPage';
