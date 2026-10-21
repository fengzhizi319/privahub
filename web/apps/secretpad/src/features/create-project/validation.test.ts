import { describe, expect, it } from 'vitest';
import {
  toValidProjectName,
  validateParticipants,
  validateProjectDescription,
  validateProjectName,
  validateTeeNode,
} from './validation';

describe('validateProjectName', () => {
  it('requires a name', () => {
    expect(validateProjectName('')).toBe('projectWizard.errors.nameRequired');
    expect(validateProjectName('   ')).toBe('projectWizard.errors.nameRequired');
  });
  it('limits length to 32', () => {
    expect(validateProjectName('a'.repeat(32))).toBeNull();
    expect(validateProjectName('a'.repeat(33))).toBe('projectWizard.errors.nameTooLong');
  });
  it('allows Chinese, letters, digits, _ and -', () => {
    expect(validateProjectName('风控_Project-01')).toBeNull();
    expect(validateProjectName('bad name')).toBe('projectWizard.errors.nameCharset');
    expect(validateProjectName('bad!')).toBe('projectWizard.errors.nameCharset');
  });
});

describe('validateProjectDescription', () => {
  it('is optional but limited to 128 chars and same charset', () => {
    expect(validateProjectDescription('')).toBeNull();
    expect(validateProjectDescription(undefined)).toBeNull();
    expect(validateProjectDescription('描述_ok')).toBeNull();
    expect(validateProjectDescription('x'.repeat(129))).toBe('projectWizard.errors.descTooLong');
    expect(validateProjectDescription('has space')).toBe('projectWizard.errors.descCharset');
  });
});

describe('validateParticipants', () => {
  it('requires 2 to 10 distinct nodes', () => {
    expect(validateParticipants(['alice'])).toBe('projectWizard.errors.tooFewNodes');
    expect(validateParticipants(['alice', 'alice'])).toBe('projectWizard.errors.tooFewNodes');
    expect(validateParticipants(['alice', 'bob'])).toBeNull();
    expect(validateParticipants(Array.from({ length: 10 }, (_, i) => `n${i}`))).toBeNull();
    expect(validateParticipants(Array.from({ length: 11 }, (_, i) => `n${i}`))).toBe('projectWizard.errors.tooManyNodes');
  });
});

describe('validateTeeNode', () => {
  it('requires a TEE node only in TEE mode', () => {
    expect(validateTeeNode('MPC', undefined)).toBeNull();
    expect(validateTeeNode('TEE', '')).toBe('projectWizard.errors.teeNodeRequired');
    expect(validateTeeNode('TEE', 'tee')).toBeNull();
  });
});

describe('toValidProjectName', () => {
  it('produces a valid name', () => {
    const name = toValidProjectName('隐私求交 (PSI) — demo!');
    expect(validateProjectName(name)).toBeNull();
    expect(toValidProjectName('x'.repeat(50))).toHaveLength(32);
  });
});
