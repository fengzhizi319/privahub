import { describe, expect, it } from 'vitest';
import { buildRegisterJson, validateRegisterJson } from './register-json';

describe('inst register json_data', () => {
  it('builds Java + Go compatible json', () => {
    const json = JSON.parse(
      buildRegisterJson({ domainId: ' alice ', token: 'tk', mode: 'p2p', port: '8083', protocol: 'notls', transPort: '1080' }),
    );
    expect(json).toEqual({ domainId: 'alice', nodeId: 'alice', token: 'tk', mode: 'p2p', port: '8083', protocol: 'notls', transPort: '1080' });
  });

  it('validates raw json', () => {
    expect(validateRegisterJson('{"domainId":"a"}')).toBeNull();
    expect(validateRegisterJson('{"nodeId":"a"}')).toBeNull();
    expect(validateRegisterJson('{}')).toBe('domainId');
    expect(validateRegisterJson('[]')).toBe('object');
    expect(validateRegisterJson('{')).toBe('json');
  });
});
