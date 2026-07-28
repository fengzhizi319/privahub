/**
 * DAG 模板向导状态管理 Hook。
 *
 * 职责范围：
 * 1. 维护模板选择、图名称、模板参数（节点/表/列/预测名等）表单状态。
 * 2. 根据所选模板自动加载节点数据表与列信息。
 * 3. 在创建时：先调用 `createGraph` 获取 graphId，再调用 `template.build(...)` 生成节点/边，
 *    最后把完整拓扑通过 `createGraph` 的 nodes/edges 参数一次性提交（后端支持）。
 * 4. 成功后回调 `onCreated(graphId)`，供上层刷新图列表并选中。
 *
 * 设计原则：
 * - 表单状态扁平存储为 `Record<string, unknown>`，通过模板 key 切换时重置为默认值。
 * - 两表模板（PSI/Risk/TEE）分别维护 receiver/sender 两套选择；单表模板只维护 node/table。
 * - 列选择默认全选特征列（除标签列外），简化向导使用。
 */
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Project } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { toast } from '@secretpad/design-system';
import type { AnyTemplateContribution, TemplateBuildInput } from './types';
import {
  asTyped,
  isSingleTableTemplate,
  isTwoTableTemplate,
  needsFeatureColumns,
  needsNoInputs,
  needsPredictionName,
  templateByKey,
  templatesByCategory,
} from './registry';
import { useTranslation } from '../../shared/lib/i18n';

export interface TemplateWizardFormState {
  templateKey: string;
  graphName: string;
  // 两表模板字段
  receiverNodeId: string;
  receiverTableId: string;
  receiverKey: string;
  senderNodeId: string;
  senderTableId: string;
  senderKey: string;
  // 单表模板字段
  nodeId: string;
  tableId: string;
  // 列相关
  featureColumns: string[];
  labelColumn: string;
  predictionName: string;
  qiColumns: string[];
  saColumns: string[];
  queryColumn: string;
  sanitizationColumns: string[];
}

const defaultForm: TemplateWizardFormState = {
  templateKey: 'psi',
  graphName: '',
  receiverNodeId: '',
  receiverTableId: '',
  receiverKey: '',
  senderNodeId: '',
  senderTableId: '',
  senderKey: '',
  nodeId: '',
  tableId: '',
  featureColumns: [],
  labelColumn: '',
  predictionName: 'pred',
  qiColumns: [],
  saColumns: [],
  queryColumn: '',
  sanitizationColumns: [],
};

function inferGraphName(template: AnyTemplateContribution): string {
  const now = new Date().toLocaleString();
  switch (template.metadata.key) {
    case 'psi':
      return `PSI 模板 ${now}`;
    case 'risk':
      return `二分类建模 ${now}`;
    case 'tee':
      return `TEE 二分类建模 ${now}`;
    case 'dataClassification':
      return `数据分类分级 ${now}`;
    case 'sanitization':
      return `数据脱敏 ${now}`;
    case 'kAnonymity':
      return `K-匿名 ${now}`;
    case 'lDiversity':
      return `L-多样性 ${now}`;
    case 'localDifferentialPrivacy':
      return `本地差分隐私 ${now}`;
    case 'differentialPrivacy':
      return `差分隐私查询 ${now}`;
    case 'queryObfuscation':
      return `查询混淆 ${now}`;
    case 'blank':
      return `空白图 ${now}`;
    default:
      return `模板图 ${now}`;
  }
}

export function useTemplateWizard(project: Project | undefined, onCreated: (graphId: string) => void) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const [form, setForm] = useState<TemplateWizardFormState>({ ...defaultForm });

  const projectNodes = useMemo(() => project?.nodes ?? [], [project]);

  const selectedTemplate = useMemo(
    () => templateByKey(form.templateKey) || templateByKey('blank')!,
    [form.templateKey]
  );

  const categories = useMemo(() => templatesByCategory(), []);

  const receiverTablesQuery = useQuery({
    queryKey: ['node-datatables', form.receiverNodeId],
    queryFn: () => apiClient.getDataTables(form.receiverNodeId),
    enabled: !!form.receiverNodeId && isTwoTableTemplate(selectedTemplate),
  });
  const senderTablesQuery = useQuery({
    queryKey: ['node-datatables', form.senderNodeId],
    queryFn: () => apiClient.getDataTables(form.senderNodeId),
    enabled: !!form.senderNodeId && isTwoTableTemplate(selectedTemplate),
  });
  const singleTablesQuery = useQuery({
    queryKey: ['node-datatables', form.nodeId],
    queryFn: () => apiClient.getDataTables(form.nodeId),
    enabled: !!form.nodeId && isSingleTableTemplate(selectedTemplate),
  });

  const receiverTables = useMemo(() => receiverTablesQuery.data ?? [], [receiverTablesQuery.data]);
  const senderTables = useMemo(() => senderTablesQuery.data ?? [], [senderTablesQuery.data]);
  const singleTables = useMemo(() => singleTablesQuery.data ?? [], [singleTablesQuery.data]);

  const receiverTable = useMemo(
    () => receiverTables.find((t) => t.tableId === form.receiverTableId),
    [receiverTables, form.receiverTableId]
  );
  const senderTable = useMemo(
    () => senderTables.find((t) => t.tableId === form.senderTableId),
    [senderTables, form.senderTableId]
  );
  const singleTable = useMemo(
    () => singleTables.find((t) => t.tableId === form.tableId),
    [singleTables, form.tableId]
  );

  const allColumns = useMemo(() => {
    if (isTwoTableTemplate(selectedTemplate)) {
      return receiverTable?.columns || senderTable?.columns || [];
    }
    if (isSingleTableTemplate(selectedTemplate)) {
      return singleTable?.columns || [];
    }
    return [];
  }, [selectedTemplate, receiverTable, senderTable, singleTable]);

  /**
   * 当用户切换模板时，根据模板需求重置/推导表单默认值。
   * - 两表模板：默认填入项目前两个节点；图名称按模板类型生成。
   * - 单表模板：默认选择项目第一个节点；特征列默认全选。
   */
  useEffect(() => {
    if (!isOpen) return;
    setForm(() => {
      const next: TemplateWizardFormState = {
        ...defaultForm,
        templateKey: selectedTemplate.metadata.key,
        graphName: inferGraphName(selectedTemplate),
      };
      if (isTwoTableTemplate(selectedTemplate)) {
        const [first, second] = projectNodes;
        if (first) next.receiverNodeId = first.nodeId;
        if (second) next.senderNodeId = second.nodeId;
      } else if (isSingleTableTemplate(selectedTemplate)) {
        const [first] = projectNodes;
        if (first) next.nodeId = first.nodeId;
      }
      return next;
    });
    // 当切换模板时清除旧的列选择，保留新默认值在后续数据加载后设置
  }, [isOpen, selectedTemplate, selectedTemplate.metadata.key, projectNodes]);

  /**
   * 当数据表列加载完成且特征列尚未选择时，默认全选除标签列外的所有列。
   */
  useEffect(() => {
    if (!isOpen || !needsFeatureColumns(selectedTemplate)) return;
    const numericCols = allColumns.filter((c) => c.type === 'float' || c.type === 'double' || c.type === 'int' || c.type === 'integer' || c.type === 'long').map((c) => c.name);
    const candidates = numericCols.length > 0 ? numericCols : allColumns.map((c) => c.name);
    if (form.featureColumns.length === 0 && candidates.length > 0) {
      setForm((prev) => ({ ...prev, featureColumns: candidates }));
    }
  }, [isOpen, selectedTemplate, selectedTemplate.metadata.key, allColumns, form.featureColumns.length]);

  const setField = useCallback(<K extends keyof TemplateWizardFormState>(field: K, value: TemplateWizardFormState[K]) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  }, []);

  const selectTemplate = useCallback((key: string) => {
    setForm((prev) => ({ ...prev, templateKey: key }));
  }, []);

  const open = useCallback(() => {
    if (!project || project.nodes.length === 0) {
      toast.error(t('projects.noProjects'));
      return;
    }
    setForm({ ...defaultForm, templateKey: 'psi' });
    setIsOpen(true);
  }, [project, t]);

  const close = useCallback(() => {
    setIsOpen(false);
  }, []);

  const buildConfigs = useCallback((): Record<string, unknown> => {
    const base: Record<string, unknown> = {
      graphId: '', // 创建后回填
    };
    if (isTwoTableTemplate(selectedTemplate)) {
      base.receiverNodeId = form.receiverNodeId;
      base.senderNodeId = form.senderNodeId;
      base.receiverTableId = form.receiverTableId;
      base.senderTableId = form.senderTableId;
      base.receiverKey = form.receiverKey;
      base.senderKey = form.senderKey;
      if (needsFeatureColumns(selectedTemplate)) {
        base.featureSelects = { ss: form.featureColumns };
        base.labelSelects = { s: form.labelColumn };
      }
      if (needsPredictionName(selectedTemplate)) {
        base.pred = { s: form.predictionName || 'pred' };
      }
    } else if (isSingleTableTemplate(selectedTemplate)) {
      base.nodeId = form.nodeId;
      base.tableId = form.tableId;
      switch (selectedTemplate.metadata.key) {
        case 'kAnonymity':
        case 'lDiversity':
          base.qiCols = form.qiColumns;
          base.saCols = form.saColumns;
          break;
        case 'localDifferentialPrivacy':
          base.queryCol = form.queryColumn;
          break;
        case 'sanitization':
          base.sanitizationCols = form.sanitizationColumns;
          break;
      }
    }
    return base;
  }, [form, selectedTemplate]);

  const createMutation = useMutation({
    mutationFn: async () => {
      if (!project) throw new Error(t('projects.selectProject'));
      const name = form.graphName.trim() || inferGraphName(selectedTemplate);
      const graphId = await apiClient.createGraph({ projectId: project.projectId, name });
      const configs = buildConfigs();
      configs.graphId = graphId;
      const typed = asTyped<Record<string, unknown>>(selectedTemplate);
      const { nodes, edges } = typed.build({ graphId, configs } as TemplateBuildInput<Record<string, unknown>>);
      // 后端 createGraph 支持直接传入 nodes/edges，但之前已经创建了空图，需要更新。
      await apiClient.updateGraph(project.projectId, graphId, nodes, edges);
      return graphId;
    },
    onSuccess: (graphId) => {
      close();
      queryClient.invalidateQueries({ queryKey: ['graphs', project?.projectId] });
      queryClient.invalidateQueries({ queryKey: ['graph-detail', project?.projectId, graphId] });
      onCreated(graphId);
      toast.success(t('dag.templateCreated'));
    },
    onError: (e) => {
      toast.error(e instanceof Error ? e.message : String(e));
    },
  });

  /**
   * 校验当前表单是否满足创建条件。
   * 两表模板需要 receiver/sender 节点、表、关联键；
   * 单表模板需要节点、表；
   * 空白/查询混淆只需图名称。
   */
  const isValid = useMemo(() => {
    if (!form.graphName.trim()) return false;
    if (needsNoInputs(selectedTemplate)) return true;
    if (isTwoTableTemplate(selectedTemplate)) {
      if (!form.receiverNodeId || !form.receiverTableId || !form.receiverKey) return false;
      if (!form.senderNodeId || !form.senderTableId || !form.senderKey) return false;
      if (needsFeatureColumns(selectedTemplate)) {
        if (form.featureColumns.length === 0 || !form.labelColumn) return false;
      }
      return true;
    }
    if (isSingleTableTemplate(selectedTemplate)) {
      return Boolean(form.nodeId && form.tableId);
    }
    return true;
  }, [form, selectedTemplate]);

  return {
    isOpen,
    open,
    close,
    form,
    setField,
    selectTemplate,
    selectedTemplate,
    categories,
    projectNodes,
    receiverTables,
    senderTables,
    singleTables,
    receiverTable,
    senderTable,
    singleTable,
    allColumns,
    createMutation,
    isValid,
  };
}

export type UseTemplateWizardReturn = ReturnType<typeof useTemplateWizard>;
