import { describe, expect, it } from 'vitest';
import { validateProjectName } from '../../features/create-project/validation';
import { templateByKey } from '../../features/dag-templates/registry';
import { scenePreset, scenes } from './scenes';

describe('privacy scenes', () => {
  it('maps every scene to a registered template', () => {
    scenes.forEach((s) => expect(templateByKey(s.templateKey)).toBeDefined());
  });
  it('builds a preset with alice/bob and a valid name', () => {
    const preset = scenePreset(scenes[0], '隐私求交 (PSI)', { supportsMpc: true, supportsTee: true }, new Date(2026, 0, 2, 3, 4));
    expect(preset).toMatchObject({ templateKey: 'psi', computeMode: 'MPC', nodeIds: ['alice', 'bob'] });
    expect(validateProjectName(preset!.name!)).toBeNull();
    expect(preset!.name).toMatch(/-01020304$/);
  });
  it('picks TEE for TEE-only templates and null when unsupported', () => {
    const tee = scenes.find((s) => s.key === 'tee')!;
    expect(scenePreset(tee, 'TEE', { supportsMpc: true, supportsTee: true })?.computeMode).toBe('TEE');
    expect(scenePreset(tee, 'TEE', { supportsMpc: true, supportsTee: false })).toBeNull();
  });
});
