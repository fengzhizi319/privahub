/**
 * Create-project wizard (port of legacy `modules/create-project`).
 *
 * Step 1 — template (DAG template registry, filtered by compute mode; blank allowed)
 * Step 2 — basic info (name / description / compute mode / compute func)
 * Step 3 — participants (2~10 Ready nodes, optional datatable per node, TEE node for TEE mode)
 *
 * Submission is delegated to `createProjectWithSetup` (see service.ts); on
 * success the wizard navigates to `/dag?projectId=&graphId=`.
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, FormField, Input, Modal, RadioGroup, Select, Steps, Textarea, toast } from '@secretpad/design-system';
import { apiClient, listNodesJava, listTeeNodesJava } from '@secretpad/api-client';
import type { NodeVOJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { usePlatform } from '@/shared/lib/platform';
import { isSingleTableTemplate, isTwoTableTemplate, templateByKey } from '../dag-templates/registry';
import { createProjectWithSetup } from './service';
import { selectableNodes } from './selectable-nodes';
import type { CreateProjectResult } from './service';
import {
  BLANK_TEMPLATE_KEY,
  COMPUTE_FUNC_OPTIONS,
  ComputeFunc,
  templateSupportsMode,
  templatesForMode,
} from './templates';
import type { ComputeMode } from './templates';
import {
  MAX_PARTICIPANTS,
  PROJECT_DESC_MAX,
  PROJECT_NAME_MAX,
  validateParticipants,
  validateProjectDescription,
  validateProjectName,
  validateTeeNode,
} from './validation';

export interface CreateProjectPreset {
  templateKey?: string;
  computeMode?: ComputeMode;
  nodeIds?: string[];
  name?: string;
  description?: string;
}

export interface CreateProjectWizardProps {
  isOpen: boolean;
  onClose: () => void;
  preset?: CreateProjectPreset;
  /** Called after success; by default the wizard navigates to the DAG editor. */
  onCreated?: (result: CreateProjectResult) => void;
}

export const CreateProjectWizard: React.FC<CreateProjectWizardProps> = ({ isOpen, onClose, preset, onCreated }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const platform = usePlatform();

  const modeOptions = useMemo(() => {
    const opts: { value: ComputeMode; label: string }[] = [];
    if (platform.supportsMpc) opts.push({ value: 'MPC', label: t('projectWizard.modeMpc') });
    if (platform.supportsTee) opts.push({ value: 'TEE', label: t('projectWizard.modeTee') });
    return opts;
  }, [platform.supportsMpc, platform.supportsTee, t]);
  const defaultMode: ComputeMode = platform.supportsMpc ? 'MPC' : 'TEE';

  const [step, setStep] = useState(0);
  const [templateKey, setTemplateKey] = useState<string>(BLANK_TEMPLATE_KEY);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [computeMode, setComputeMode] = useState<ComputeMode>(defaultMode);
  const [computeFunc, setComputeFunc] = useState<string>(ComputeFunc.DAG);
  const [nodeIds, setNodeIds] = useState<string[]>([]);
  /** nodeId → datatableId; `''` means explicitly "no table", missing means untouched. */
  const [tables, setTables] = useState<Record<string, string>>({});
  const [teeNodeId, setTeeNodeId] = useState('');
  const [nodeSearch, setNodeSearch] = useState('');
  const [errors, setErrors] = useState<Record<string, string | null>>({});

  // Reset state whenever the wizard is (re)opened.
  useEffect(() => {
    if (!isOpen) return;
    const mode = preset?.computeMode && modeOptions.some((o) => o.value === preset.computeMode) ? preset.computeMode : defaultMode;
    setComputeMode(mode);
    setTemplateKey(preset?.templateKey || BLANK_TEMPLATE_KEY);
    setName(preset?.name || '');
    setDescription(preset?.description || '');
    setComputeFunc(ComputeFunc.DAG);
    setNodeIds(preset?.nodeIds || []);
    setTables({});
    setTeeNodeId('');
    setNodeSearch('');
    setErrors({});
    setStep(preset?.templateKey ? 1 : 0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen]);

  const nodesQuery = useQuery({
    queryKey: ['project-wizard-nodes'],
    queryFn: listNodesJava,
    enabled: isOpen,
  });
  const allNodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const candidates = useMemo(() => selectableNodes(allNodes), [allNodes]);

  const teeQuery = useQuery({
    queryKey: ['project-wizard-tee-nodes'],
    queryFn: listTeeNodesJava,
    enabled: isOpen && computeMode === 'TEE',
  });
  const teeNodes = useMemo(() => teeQuery.data ?? [], [teeQuery.data]);
  useEffect(() => {
    if (computeMode === 'TEE' && !teeNodeId && teeNodes[0]) setTeeNodeId(teeNodes[0].nodeId);
  }, [computeMode, teeNodeId, teeNodes]);

  // EDGE account on CENTER: own node is always a participant (added by backend).
  const lockedNodeId = platform.isEdgeAccountOnCenter ? platform.ownerId : '';
  useEffect(() => {
    if (!isOpen) return;
    if (lockedNodeId && !nodeIds.includes(lockedNodeId)) setNodeIds((prev) => [lockedNodeId, ...prev]);
  }, [isOpen, lockedNodeId, nodeIds]);

  // Drop preset nodes that are not selectable (e.g. alice/bob not present).
  useEffect(() => {
    if (!nodesQuery.data) return;
    const ok = new Set(candidates.map((n) => n.nodeId));
    setNodeIds((prev) => {
      const next = prev.filter((id) => ok.has(id) || id === lockedNodeId);
      return next.length === prev.length ? prev : next;
    });
  }, [nodesQuery.data, candidates, lockedNodeId]);

  // Datatables of each selected node: use NodeVO.datatables, else datatable/list.
  const tableQueries = useQueries({
    queries: nodeIds.map((id) => ({
      queryKey: ['datatables', id],
      queryFn: () => apiClient.getDataTables(id),
      enabled: isOpen && step === 2,
    })),
  });
  const tableOptions = useMemo(() => {
    const map: Record<string, { value: string; label: string }[]> = {};
    nodeIds.forEach((id, idx) => {
      const node = allNodes.find((n) => n.nodeId === id);
      const fromNode = (node?.datatables || [])
        .filter((d) => d.datatableId)
        .map((d) => ({ value: d.datatableId!, label: d.datatableName || d.datatableId! }));
      const fetched = (tableQueries[idx]?.data || []).map((d) => ({ value: d.tableId, label: d.tableName || d.tableId }));
      map[id] = fromNode.length > 0 ? fromNode : fetched;
    });
    return map;
  }, [nodeIds, allNodes, tableQueries]);

  const template = templateByKey(templateKey);
  const templateNeedsTables = !!template && (isTwoTableTemplate(template) || isSingleTableTemplate(template));

  // Quick authorization: default to each node's first table when the template needs data.
  useEffect(() => {
    if (!templateNeedsTables) return;
    setTables((prev) => {
      let changed = false;
      const next = { ...prev };
      nodeIds.forEach((id) => {
        if (next[id] === undefined && tableOptions[id]?.[0]) {
          next[id] = tableOptions[id][0].value;
          changed = true;
        }
      });
      return changed ? next : prev;
    });
  }, [templateNeedsTables, nodeIds, tableOptions]);

  const availableTemplates = useMemo(() => templatesForMode(computeMode), [computeMode]);

  const changeMode = (mode: ComputeMode) => {
    setComputeMode(mode);
    const current = templateByKey(templateKey);
    if (current && !templateSupportsMode(current, mode)) setTemplateKey(BLANK_TEMPLATE_KEY);
  };

  const validateStep = (s: number): boolean => {
    if (s === 0) return !!templateKey;
    if (s === 1) {
      const next = {
        name: validateProjectName(name),
        description: validateProjectDescription(description),
      };
      setErrors((prev) => ({ ...prev, ...next }));
      return !next.name && !next.description;
    }
    const next = {
      nodes: validateParticipants(nodeIds),
      tee: validateTeeNode(computeMode, teeNodeId),
    };
    setErrors((prev) => ({ ...prev, ...next }));
    return !next.nodes && !next.tee;
  };

  const mutation = useMutation({
    mutationFn: () =>
      createProjectWithSetup(
        {
          name,
          description,
          computeMode,
          computeFunc,
          teeNodeId: computeMode === 'TEE' ? teeNodeId : undefined,
          templateKey,
          graphName: template ? `${t(`dag.templateName.${template.metadata.key}`)}` : undefined,
          participants: nodeIds.map((id) => ({
            nodeId: id,
            instId: allNodes.find((n) => n.nodeId === id)?.instId,
            datatableId: tables[id] || undefined,
          })),
        },
        { platformType: platform.platformType, ownerType: platform.ownerType, ownerId: platform.ownerId }
      ),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      if (result.warnings.length > 0) {
        toast.warning(
          t('projectWizard.partialFailure', {
            detail: result.warnings.map((w) => `${t(`projectWizard.stepName.${w.step}`)} ${w.target}: ${w.message}`).join('; '),
          }),
          8000
        );
      } else {
        toast.success(t('projects.createSuccess'));
      }
      onClose();
      if (onCreated) onCreated(result);
      else navigate({ to: '/dag', search: { projectId: result.projectId, graphId: result.graphId } });
    },
    onError: (e) => {
      toast.error(`${t('projects.createError')}: ${e instanceof Error ? e.message : String(e)}`);
    },
  });

  const next = () => {
    if (!validateStep(step)) return;
    if (step < 2) setStep(step + 1);
    else mutation.mutate();
  };

  const filteredCandidates = candidates.filter((n) => {
    const q = nodeSearch.trim().toLowerCase();
    if (!q) return true;
    return `${n.nodeName || ''} ${n.nodeId} ${n.instName || ''}`.toLowerCase().includes(q);
  });

  const toggleNode = (id: string) => {
    if (id === lockedNodeId) {
      toast.warning(t('projectWizard.ownNodeLocked'));
      return;
    }
    setNodeIds((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id);
      if (prev.length >= MAX_PARTICIPANTS) return prev;
      return [...prev, id];
    });
    setErrors((prev) => ({ ...prev, nodes: null }));
  };

  const nodeLabel = (n: NodeVOJava) =>
    platform.isAutonomy && n.instName ? `${n.instName}-${n.nodeName || n.nodeId} (${n.nodeId})` : `${n.nodeName || n.nodeId} (${n.nodeId})`;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('projectWizard.title')}
      width="max-w-3xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          {step > 0 && (
            <Button variant="outline" onClick={() => setStep(step - 1)} disabled={mutation.isPending}>
              {t('projectWizard.prev')}
            </Button>
          )}
          <Button variant="primary" onClick={next} loading={mutation.isPending} data-tour="wizard-submit">
            {step < 2 ? t('projectWizard.next') : t('projectWizard.submit')}
          </Button>
        </>
      }
    >
      <div className="space-y-5 text-xs max-h-[65vh] overflow-y-auto pr-1">
        <Steps
          steps={[t('projectWizard.stepTemplate'), t('projectWizard.stepBasic'), t('projectWizard.stepNodes')]}
          current={step}
        />

        {step === 0 && (
          <div className="space-y-3">
            <FormField label={t('projects.modeLabel')} required>
              <RadioGroup options={modeOptions} value={computeMode} onChange={(v) => changeMode(v as ComputeMode)} />
            </FormField>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-3" data-tour="wizard-templates">
              {availableTemplates.map((tpl) => {
                const checked = tpl.metadata.key === templateKey;
                return (
                  <button
                    type="button"
                    key={tpl.metadata.key}
                    onClick={() => setTemplateKey(tpl.metadata.key)}
                    className={`text-left p-3 rounded-lg border transition-colors ${
                      checked
                        ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/40'
                        : 'border-gray-200 dark:border-gray-700 hover:border-blue-400'
                    }`}
                  >
                    <div className="font-semibold text-gray-900 dark:text-gray-100 flex items-center justify-between gap-2">
                      <span>{t(`dag.templateName.${tpl.metadata.key}`)}</span>
                      {checked && <span className="text-blue-600">✓</span>}
                    </div>
                    <div className="text-[11px] text-gray-500 mt-1 line-clamp-2">{t(`dag.templateDesc.${tpl.metadata.key}`)}</div>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {step === 1 && (
          <div className="space-y-4">
            <FormField
              label={t('projects.nameLabel')}
              required
              error={errors.name ? t(errors.name, { max: PROJECT_NAME_MAX }) : undefined}
              help={t('projectWizard.nameHelp', { max: PROJECT_NAME_MAX })}
            >
              <Input
                value={name}
                maxLength={PROJECT_NAME_MAX + 8}
                invalid={!!errors.name}
                placeholder={t('projectWizard.namePlaceholder')}
                onChange={(e) => {
                  setName(e.target.value);
                  setErrors((p) => ({ ...p, name: null }));
                }}
                autoFocus
              />
            </FormField>
            <FormField
              label={t('projects.descLabel')}
              error={errors.description ? t(errors.description, { max: PROJECT_DESC_MAX }) : undefined}
            >
              <Textarea
                value={description}
                rows={3}
                invalid={!!errors.description}
                placeholder={t('projectWizard.descPlaceholder', { max: PROJECT_DESC_MAX })}
                onChange={(e) => {
                  setDescription(e.target.value);
                  setErrors((p) => ({ ...p, description: null }));
                }}
              />
            </FormField>
            <FormField label={t('projects.modeLabel')} required help={t('projectWizard.modeHelp')}>
              <RadioGroup options={modeOptions} value={computeMode} onChange={(v) => changeMode(v as ComputeMode)} />
            </FormField>
            <FormField label={t('projectWizard.computeFunc')} required>
              <RadioGroup
                options={COMPUTE_FUNC_OPTIONS.map((f) => ({ value: f, label: t(`projectWizard.computeFuncName.${f}`) }))}
                value={computeFunc}
                onChange={setComputeFunc}
              />
            </FormField>
            <div className="text-[11px] text-gray-500">
              {t('projectWizard.selectedTemplate')}: <b>{t(`dag.templateName.${templateKey}`)}</b>
            </div>
          </div>
        )}

        {step === 2 && (
          <div className="space-y-4">
            {computeMode === 'TEE' && (
              <FormField label={t('projectWizard.teeNode')} required error={errors.tee ? t(errors.tee) : undefined}>
                <Select
                  value={teeNodeId}
                  placeholder={teeQuery.isLoading ? t('common.loading') : t('projectWizard.teeNodePlaceholder')}
                  options={teeNodes.map((n) => ({ value: n.nodeId, label: `${n.nodeName || n.nodeId} (${n.nodeId})` }))}
                  onChange={(v) => {
                    setTeeNodeId(v);
                    setErrors((p) => ({ ...p, tee: null }));
                  }}
                />
              </FormField>
            )}

            <FormField
              label={`${t('projectWizard.participants')} (${nodeIds.length}/${MAX_PARTICIPANTS})`}
              required
              help={t('projectWizard.participantsHelp')}
              error={errors.nodes ? t(errors.nodes) : undefined}
            >
              <div className="space-y-2">
                <Input
                  value={nodeSearch}
                  placeholder={t('projectWizard.searchNode')}
                  onChange={(e) => setNodeSearch(e.target.value)}
                />
                {nodesQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
                {nodesQuery.error && <div className="text-red-500">{(nodesQuery.error as Error).message}</div>}
                {!nodesQuery.isLoading && candidates.length === 0 && (
                  <div className="text-gray-400">{t('projectWizard.noReadyNodes')}</div>
                )}
                <div className="flex flex-wrap gap-2" data-tour="wizard-nodes">
                  {filteredCandidates.map((n) => {
                    const checked = nodeIds.includes(n.nodeId);
                    const disabled = !checked && nodeIds.length >= MAX_PARTICIPANTS;
                    return (
                      <label
                        key={n.nodeId}
                        className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg border cursor-pointer select-none ${
                          checked
                            ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300'
                            : 'border-gray-200 dark:border-gray-700 text-gray-700 dark:text-gray-300'
                        } ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
                      >
                        <input
                          type="checkbox"
                          className="accent-blue-600"
                          checked={checked}
                          disabled={disabled || n.nodeId === lockedNodeId}
                          onChange={() => toggleNode(n.nodeId)}
                        />
                        {n.type === 'embedded' && (
                          <span className="px-1 rounded bg-cyan-100 dark:bg-cyan-900/50 text-cyan-700 dark:text-cyan-300 text-[10px]">
                            {t('projectWizard.embedded')}
                          </span>
                        )}
                        {nodeLabel(n)}
                      </label>
                    );
                  })}
                </div>
              </div>
            </FormField>

            {nodeIds.length > 0 && (
              <FormField label={t('projectWizard.quickAuth')} help={t('projectWizard.quickAuthHelp')}>
                <div className="space-y-2">
                  {nodeIds.map((id) => {
                    const node = allNodes.find((n) => n.nodeId === id);
                    const opts = tableOptions[id] || [];
                    return (
                      <div key={id} className="flex items-center gap-3">
                        <span className="w-40 truncate font-medium text-gray-700 dark:text-gray-300">
                          {node?.nodeName || id}
                        </span>
                        <Select
                          className="flex-1"
                          value={tables[id] ?? ''}
                          options={[{ value: '', label: t('projectWizard.noTable') }, ...opts]}
                          onChange={(v) => setTables((p) => ({ ...p, [id]: v }))}
                        />
                      </div>
                    );
                  })}
                  {nodeIds.every((id) => (tableOptions[id] || []).length === 0) && (
                    <div className="text-gray-400">{t('projectWizard.noTablesHint')}</div>
                  )}
                </div>
              </FormField>
            )}
          </div>
        )}
      </div>
    </Modal>
  );
};
