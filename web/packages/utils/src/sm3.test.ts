import { describe, it, expect } from 'vitest';
import { sm3, sm3Bytes } from './sm3';

// Reference vectors: GB/T 32905-2016 appendix A plus values produced by the
// legacy frontend's `sm-crypto@0.5.7` sm3() for compatibility.
describe('sm3', () => {
  it.each([
    ['abc', '66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0'],
    ['abcd'.repeat(16), 'debe9ff92275b8a138604889c18e5a4d6fdb70e5387e5765293dcba39c0c5732'],
    ['12345678', '0fffff81e971fa3f09107abf77931463fc0710bfb8962efeae3d5654b073bb0c'],
    ['Admin12345', '8f8bce390b1b67503a07d642de612a617d4a663417ff0b386276d7239845bbf3'],
    ['中文密码', '37ec5a40b86472fbb0cb4fdc702eff2256a0e614e5d9f147b452e75b42fc6cda'],
    ['', '1ab21d8355cfa17f8e61194831e81a8f22bec8c728fefb747ed035eb5082aa2b'],
  ])('hashes %j like sm-crypto', (input, expected) => {
    expect(sm3(input)).toBe(expected);
  });

  it('returns 32 raw bytes', () => {
    expect(sm3Bytes(new Uint8Array([1, 2, 3]))).toHaveLength(32);
  });

  it('handles messages spanning multiple blocks', () => {
    expect(sm3('x'.repeat(200))).toMatch(/^[0-9a-f]{64}$/);
  });
});
