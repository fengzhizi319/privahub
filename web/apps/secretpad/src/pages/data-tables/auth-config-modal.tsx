/**
 * 数据表授权配置（旧前端 `ProjectAuthConfigDrawer`）。
 *
 * - ADD：选择关联项目（P2P/AUTONOMY 仅列出已通过且包含该节点的项目），按字段配置后 `project/datatable/add`；
 * - EDIT：`project/datatable/get` 读取已有配置，修改后 `project/update/tableConfig`；
 * - 字段配置：关联键 / 分组列 / 标签列 / 保护开关（SCQL 联合分析生效）。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Button, Empty, Modal, Select, toast } from '@secretpad/design-system';
import {
  authDatatableToProjectJava,
  getProjectDatatableConfigJava,
  listAuthorizableProjectsJava,
  updateProjectTableConfigJava,
} from '@secretpad/api-client';
import type { DatatableAuthProjectJava, DatatableRowJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { applyColumnConfigChange, mergeColumnConfigs, toConfigParams } from './auth-model';
import type { ColumnConfig, ColumnConfigKey } from './auth-model';

export interface AuthConfigModalProps {
  isOpen: boolean;
  mode: 'ADD' | 'EDIT';
  table: DatatableRowJava;
  nodeId: string;
  project?: DatatableAuthProjectJava;
  p2p: boolean;
  onClose: () => void;
  onDone: () => void;
}

export const AuthConfigModal: React.FC<AuthConfigModalProps> = ({ isOpen, mode, table, nodeId, project, p2p, onClose, onDone }) => {
  const { t } = useTranslation();
  const [projectId, setProjectId] = useState('');
  const [fields, setFields] = useState<ColumnConfig[]>([]);
  const [showHelp, setShowHelp] = useState(false);

  const projectsQuery = useQuery({
    queryKey: ['authorizable-projects', p2p, nodeId],
    queryFn: () => listAuthorizableProjectsJava({ p2p, nodeId }),
    enabled: isOpen && mode === 'ADD',
  });
  const authorized = useMemo(() => new Set((table.authProjects || []).map((a) => a.projectId)), [table.authProjects]);
  const projectOptions = (projectsQuery.data || [])
    .filter((p) => !authorized.has(p.projectId))
    .map((p) => ({ value: p.projectId, label: `[${p.computeMode || 'MPC'}] ${p.projectName || p.projectId}` }));

  const configQuery = useQuery({
    queryKey: ['project-datatable-config', project?.projectId, nodeId, table.datatableId],
    queryFn: () =>
      getProjectDatatableConfigJava({ projectId: project?.projectId, nodeId, datatableId: table.datatableId, type: table.type }),
    enabled: isOpen && mode === 'EDIT' && !!project?.projectId,
  });

  useEffect(() => {
    if (!isOpen) return;
    setProjectId('');
    setFields(mode === 'ADD' ? mergeColumnConfigs(table.schema) : []);
  }, [isOpen, mode, table.schema]);

  useEffect(() => {
    if (mode === 'EDIT' && configQuery.data) setFields(mergeColumnConfigs(table.schema, configQuery.data));
  }, [mode, configQuery.data, table.schema]);

  const saveMutation = useMutation({
    mutationFn: () => {
      const body = {
        projectId: mode === 'ADD' ? projectId : project?.projectId,
        nodeId,
        datatableId: table.datatableId,
        configs: toConfigParams(fields),
        type: table.type,
      };
      return mode === 'ADD' ? authDatatableToProjectJava(body) : updateProjectTableConfigJava(body);
    },
    onSuccess: () => {
      toast.success(mode === 'ADD' ? t('dataTables.authSuccess') : t('dtAuth.updateSuccess'));
      onDone();
      onClose();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const change = (colName: string, key: ColumnConfigKey, value: boolean) =>
    setFields((prev) => applyColumnConfigChange(prev, colName, key, value));

  const showFields = mode === 'EDIT' || !!projectId;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      width="max-w-4xl"
      title={mode === 'ADD' ? t('dtAuth.addTitle') : t('dtAuth.editTitle', { name: project?.name || project?.projectId || '' })}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            loading={saveMutation.isPending}
            disabled={(mode === 'ADD' && !projectId) || fields.length === 0}
            onClick={() => saveMutation.mutate()}
          >
            {t('common.confirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-3 text-xs max-h-[65vh] overflow-y-auto pr-1">
        {mode === 'ADD' && (
          <div className="flex items-center gap-2">
            <span className="shrink-0 font-semibold">{t('dtAuth.project')}</span>
            <Select
              aria-label={t('dtAuth.project')}
              value={projectId}
              placeholder={t('dsForm.selectPlaceholder')}
              options={projectOptions}
              onChange={setProjectId}
              className="max-w-md"
            />
          </div>
        )}
        <div className="px-3 py-2 rounded-lg bg-amber-50 dark:bg-amber-950/30 text-amber-700 dark:text-amber-300">
          {t('dtAuth.scqlOnly')}
        </div>
        <div className="flex items-center justify-between">
          <span className="font-semibold">{t('dtAuth.safeSetting')}</span>
          <Button size="sm" variant="link" onClick={() => setShowHelp((v) => !v)}>
            {t('dtAuth.safeHelp')}
          </Button>
        </div>
        {showHelp && (
          <ul className="list-disc pl-5 space-y-1 text-gray-600 dark:text-gray-300">
            <li>{t('dtAuth.case1')}</li>
            <li>{t('dtAuth.case2')}</li>
            <li>{t('dtAuth.case3')}</li>
            <li>{t('dtAuth.case4')}</li>
          </ul>
        )}

        {!showFields && <Empty>{t('dtAuth.selectProjectFirst')}</Empty>}
        {showFields && configQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
        {showFields && (
          <div className="overflow-x-auto">
          <table className="w-full text-left">
            <thead className="bg-gray-50 dark:bg-gray-800 text-gray-500 font-semibold">
              <tr>
                <th className="p-2">{t('dataTables.columnName')}</th>
                <th className="p-2">{t('dataTables.dataType')}</th>
                <th className="p-2">{t('dataTables.columnDescription')}</th>
                <th className="p-2 text-center">{t('dataTables.associateKey')}</th>
                <th className="p-2 text-center">{t('dtAuth.groupKey')}</th>
                <th className="p-2 text-center">{t('dataTables.labelKey')}</th>
                <th className="p-2">{t('dtAuth.protection')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {fields.map((f) => (
                <tr key={f.colName}>
                  <td className="p-2 font-mono">{f.colName || '--'}</td>
                  <td className="p-2 font-mono text-gray-500">{f.colType || '--'}</td>
                  <td className="p-2 text-gray-500">{f.colComment || '--'}</td>
                  {(['isAssociateKey', 'isGroupKey', 'isLabelKey'] as const).map((k) => (
                    <td key={k} className="p-2 text-center">
                      <input
                        type="checkbox"
                        aria-label={`${f.colName} ${k}`}
                        checked={!!f[k]}
                        onChange={(e) => change(f.colName, k, e.target.checked)}
                      />
                    </td>
                  ))}
                  <td className="p-2">
                    <label className="inline-flex items-center gap-1.5 cursor-pointer">
                      <input
                        type="checkbox"
                        role="switch"
                        aria-label={`${f.colName} isProtection`}
                        checked={!!f.isProtection}
                        onChange={(e) => change(f.colName, 'isProtection', e.target.checked)}
                      />
                      <span className={f.isProtection ? 'text-emerald-600' : 'text-rose-500'} title={f.isProtection ? '' : t('dtAuth.unprotectedTip')}>
                        {f.isProtection ? t('dtAuth.protected') : t('dtAuth.unprotected')}
                      </span>
                    </label>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        )}
      </div>
    </Modal>
  );
};
