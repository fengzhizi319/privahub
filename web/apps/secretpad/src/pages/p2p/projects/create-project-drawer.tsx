/**
 * P2P create project (legacy `create-project/p2p-create-project`).
 *
 * 1. `p2p/project/create {name, description, computeMode, computeFunc}`
 * 2. `approval/create {initiatorId, voteType: PROJECT_CREATE, voteConfig: {projectId,
 *    participants, participantNodeInstVOS}}`
 *
 * My nodes come from `inst/node/list` (Ready only); invitable nodes per my node
 * are the `srcNode`s of `nodeRoute/page` routes with dst = my node and status Succeeded.
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  Button,
  CheckboxGroup,
  Drawer,
  FormField,
  Input,
  RadioGroup,
  Select,
  Textarea,
  toast,
} from '@secretpad/design-system';
import {
  createApprovalJava,
  createP2pProjectJava,
  listInstNodesJava,
  pageNodeRoutesJava,
} from '@secretpad/api-client';
import { useTranslation } from '../../../shared/lib/i18n';
import { usePlatform } from '../../../shared/lib/platform';
import {
  MAX_INVITEES_PER_GROUP,
  MAX_NODE_GROUPS,
  PROJECT_DESC_MAX,
  PROJECT_NAME_MAX,
  buildProjectCreateVoteConfig,
  validateNodeGroups,
  validateProjectDesc,
  validateProjectName,
  votersForNode,
  type NodeGroup,
  type VoterNode,
} from './helpers';

export const CreateP2pProjectDrawer: React.FC<{ isOpen: boolean; onClose: () => void; onCreated: () => void }> = ({
  isOpen,
  onClose,
  onCreated,
}) => {
  const { t } = useTranslation();
  const { ownerId, supportsMpc, supportsTee, deployMode } = usePlatform();
  const defaultMode = deployMode === 'TEE' ? 'TEE' : 'MPC';
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [computeMode, setComputeMode] = useState(defaultMode);
  const [groups, setGroups] = useState<NodeGroup[]>([{ nodeId: '', invitees: [] }]);
  const [submitted, setSubmitted] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setName('');
      setDescription('');
      setComputeMode(defaultMode);
      setGroups([{ nodeId: '', invitees: [] }]);
      setSubmitted(false);
    }
  }, [isOpen, defaultMode]);

  const nodesQuery = useQuery({
    queryKey: ['p2p-inst-nodes'],
    queryFn: listInstNodesJava,
    enabled: isOpen,
  });
  const routesQuery = useQuery({
    queryKey: ['p2p-node-routes', ownerId],
    queryFn: () => pageNodeRoutesJava({ page: 1, size: 1000, search: '', sort: {}, ownerId }),
    enabled: isOpen,
  });

  const myNodes = useMemo(
    () => (nodesQuery.data ?? []).filter((n) => n.nodeStatus === 'Ready' && n.nodeId),
    [nodesQuery.data],
  );
  const votersByNode = useMemo(() => {
    const routes = routesQuery.data?.list ?? [];
    const map: Record<string, VoterNode[]> = {};
    for (const n of myNodes) map[n.nodeId!] = votersForNode(routes, n.nodeId!);
    return map;
  }, [routesQuery.data, myNodes]);

  const nameError = validateProjectName(name);
  const descError = validateProjectDesc(description);
  const groupsError = validateNodeGroups(groups);

  const mutation = useMutation({
    mutationFn: async () => {
      const created = await createP2pProjectJava({
        name: name.trim(),
        description: description.trim(),
        computeMode,
        computeFunc: 'DAG',
      });
      if (!created.projectId) throw new Error(t('p2pProjects.errors.createFailed'));
      await createApprovalJava({
        initiatorId: ownerId,
        voteType: 'PROJECT_CREATE',
        voteConfig: buildProjectCreateVoteConfig(created.projectId, ownerId, groups, votersByNode),
      });
    },
    onSuccess: () => {
      toast.success(t('p2pProjects.createSuccess'));
      onCreated();
      onClose();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const submit = () => {
    setSubmitted(true);
    if (nameError || descError || groupsError) return;
    mutation.mutate();
  };

  const updateGroup = (index: number, patch: Partial<NodeGroup>) =>
    setGroups((gs) => gs.map((g, i) => (i === index ? { ...g, ...patch } : g)));

  const modeOptions = [
    ...(supportsMpc ? [{ value: 'MPC', label: t('msgCenter.computeModes.MPC') }] : []),
    ...(supportsTee ? [{ value: 'TEE', label: t('msgCenter.computeModes.TEE') }] : []),
  ];

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      width="max-w-2xl"
      title={t('p2p.createProjectTitle')}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={mutation.isPending} onClick={submit}>
            {t('common.create')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        <FormField label={t('p2p.name')} required error={submitted && nameError ? t(nameError, { max: PROJECT_NAME_MAX }) : undefined}>
          <Input
            className="w-full"
            value={name}
            maxLength={PROJECT_NAME_MAX}
            placeholder={t('p2pProjects.namePlaceholder')}
            onChange={(e) => setName(e.target.value)}
          />
        </FormField>
        <FormField label={t('p2p.description')} error={submitted && descError ? t(descError, { max: PROJECT_DESC_MAX }) : undefined}>
          <Textarea
            className="w-full"
            rows={2}
            maxLength={PROJECT_DESC_MAX}
            placeholder={t('p2pProjects.descPlaceholder')}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </FormField>
        <FormField label={t('p2p.computeFunc')} required help={t('p2pProjects.computeFuncDAGDesc')}>
          <RadioGroup
            name={t('p2p.computeFunc')}
            value="DAG"
            onChange={() => undefined}
            options={[{ value: 'DAG', label: t('msgCenter.computeFuncs.DAG') }]}
          />
        </FormField>
        <FormField label={t('p2p.computeMode')} required>
          <RadioGroup name={t('p2p.computeMode')} value={computeMode} onChange={setComputeMode} options={modeOptions} />
        </FormField>

        <FormField
          label={t('p2pProjects.nodeInfo')}
          required
          error={submitted && groupsError ? t(groupsError, { max: MAX_INVITEES_PER_GROUP }) : undefined}
        >
          <div className="p-2.5 mb-2 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900 text-amber-700 dark:text-amber-400">
            {t('p2pProjects.routeHint')}
          </div>
          {(nodesQuery.error || routesQuery.error) && (
            <div className="text-rose-500 mb-2">
              {t('common.error', { message: (nodesQuery.error || routesQuery.error)?.message || '' })}
            </div>
          )}
          <div className="space-y-3">
            {groups.map((g, i) => {
              const voters = g.nodeId ? votersByNode[g.nodeId] || [] : [];
              return (
                <div key={i} className="rounded-lg border border-gray-200 dark:border-gray-700 p-3 space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <Select
                      aria-label={t('p2pProjects.myNode')}
                      className="w-64"
                      placeholder={t('p2pProjects.myNode')}
                      value={g.nodeId}
                      onChange={(v) => updateGroup(i, { nodeId: v, invitees: [] })}
                      options={myNodes.map((n) => ({
                        value: n.nodeId!,
                        label: n.nodeName || n.nodeId!,
                        disabled: groups.some((o, oi) => oi !== i && o.nodeId === n.nodeId),
                      }))}
                    />
                    {groups.length > 1 && (
                      <Button size="sm" variant="ghost" onClick={() => setGroups((gs) => gs.filter((_, gi) => gi !== i))}>
                        {t('common.delete')}
                      </Button>
                    )}
                  </div>
                  <div className="text-gray-500">
                    {t('p2pProjects.invitees', { max: MAX_INVITEES_PER_GROUP })}
                  </div>
                  {g.nodeId && voters.length === 0 && (
                    <div className="text-gray-400">{t('p2pProjects.noVoters')}</div>
                  )}
                  <CheckboxGroup
                    options={voters.map((v) => ({
                      value: v.nodeId,
                      label: `${v.nodeName || v.nodeId}${v.instName ? ` (${v.instName})` : ''}`,
                    }))}
                    value={g.invitees}
                    max={MAX_INVITEES_PER_GROUP}
                    onChange={(invitees) => updateGroup(i, { invitees })}
                  />
                </div>
              );
            })}
          </div>
          <div className="mt-2" title={groups.length >= MAX_NODE_GROUPS ? t('p2pProjects.maxGroups', { max: MAX_NODE_GROUPS }) : undefined}>
            <Button
              size="sm"
              variant="outline"
              disabled={groups.length >= MAX_NODE_GROUPS}
              onClick={() => setGroups((gs) => [...gs, { nodeId: '', invitees: [] }])}
            >
              {t('p2pProjects.addGroup')}
            </Button>
          </div>
        </FormField>
      </div>
    </Drawer>
  );
};
