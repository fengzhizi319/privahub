import { describe, expect, it } from 'vitest';
import { encodeAuthCode, getProtocol, isValidNetAddress, parseAuthCode, stripProtocol } from './auth-code';

const full = {
  certText: 'CERT==',
  dstNodeId: 'node-b',
  name: '合作节点B',
  dstNetAddress: 'https://10.0.0.2:1080',
  instId: 'inst-b',
  instName: '机构B',
  masterNodeId: 'master-b',
};

describe('parseAuthCode', () => {
  it('parses a complete base64 JSON auth code (UTF-8 safe)', () => {
    const parsed = parseAuthCode(encodeAuthCode(full));
    expect(parsed).toEqual({
      certText: 'CERT==',
      dstNodeId: 'node-b',
      name: '合作节点B',
      dstNetAddress: '10.0.0.2:1080',
      protocol: 'https://',
      instId: 'inst-b',
      instName: '机构B',
      masterNodeId: 'master-b',
    });
  });

  it('defaults to http when the address has no protocol', () => {
    const parsed = parseAuthCode(encodeAuthCode({ ...full, dstNetAddress: '10.0.0.2:1080' }));
    expect(parsed?.protocol).toBe('http://');
    expect(parsed?.dstNetAddress).toBe('10.0.0.2:1080');
  });

  it('rejects codes missing any required field', () => {
    const { masterNodeId: _omit, ...partial } = full;
    expect(parseAuthCode(encodeAuthCode(partial))).toBeNull();
  });

  it('rejects garbage / non-JSON / empty input', () => {
    expect(parseAuthCode('')).toBeNull();
    expect(parseAuthCode('not base64 !!')).toBeNull();
    expect(parseAuthCode(btoa('hello'))).toBeNull();
  });
});

describe('protocol helpers', () => {
  it('getProtocol / stripProtocol', () => {
    expect(getProtocol('https://a:1')).toBe('https://');
    expect(getProtocol('a:1', 'mtls')).toBe('https://');
    expect(getProtocol('a:1', 'notls')).toBe('http://');
    expect(getProtocol(undefined)).toBe('http://');
    expect(stripProtocol('http://a:1')).toBe('a:1');
    expect(stripProtocol('a:1')).toBe('a:1');
  });

  it('isValidNetAddress', () => {
    expect(isValidNetAddress('127.0.0.1:1080')).toBe(true);
    expect(isValidNetAddress('bob:65535')).toBe(true);
    expect(isValidNetAddress('bob:65536')).toBe(false);
    expect(isValidNetAddress('bob')).toBe(false);
    expect(isValidNetAddress('b ob:80')).toBe(false);
  });
});
