/**
 * SM3 cryptographic hash (GB/T 32905-2016), dependency-free implementation.
 *
 * The legacy SecretPad frontend hashed login passwords with `sm-crypto`'s
 * `sm3()` before sending them to the Java backend. Accounts created by the
 * legacy platform therefore store SM3 digests; we send this value alongside
 * the SHA-256 hash (as `passwordHashSm3`) so the Go backend can verify such
 * legacy accounts. Output is a lowercase 64-char hex string, identical to
 * `sm-crypto`'s `sm3(message)` for UTF-8 string input.
 */

const IV = [
  0x7380166f, 0x4914b2b9, 0x172442d7, 0xda8a0600, 0xa96f30bc, 0x163138aa, 0xe38dee4d, 0xb0fb0e4e,
];

function rotl(x: number, n: number): number {
  const s = n % 32;
  return ((x << s) | (x >>> (32 - s))) >>> 0;
}

function p0(x: number): number {
  return (x ^ rotl(x, 9) ^ rotl(x, 17)) >>> 0;
}

function p1(x: number): number {
  return (x ^ rotl(x, 15) ^ rotl(x, 23)) >>> 0;
}

function utf8Bytes(message: string): Uint8Array {
  return new TextEncoder().encode(message);
}

/** Compute the SM3 digest of raw bytes and return 32 bytes. */
export function sm3Bytes(input: Uint8Array): Uint8Array {
  const bitLen = input.length * 8;
  // Padding: 1 bit, zeros, then 64-bit big-endian length; total length ≡ 0 mod 64 bytes.
  const padLen = ((input.length + 9 + 63) >> 6) << 6;
  const buf = new Uint8Array(padLen);
  buf.set(input);
  buf[input.length] = 0x80;
  const view = new DataView(buf.buffer);
  // High 32 bits of the bit length (messages < 512MB keep this at 0 except via division).
  view.setUint32(padLen - 8, Math.floor(bitLen / 0x100000000) >>> 0);
  view.setUint32(padLen - 4, bitLen >>> 0);

  const v = IV.slice();
  const w = new Array<number>(68);
  const w1 = new Array<number>(64);

  for (let offset = 0; offset < padLen; offset += 64) {
    for (let j = 0; j < 16; j++) w[j] = view.getUint32(offset + j * 4);
    for (let j = 16; j < 68; j++) {
      w[j] = (p1(w[j - 16] ^ w[j - 9] ^ rotl(w[j - 3], 15)) ^ rotl(w[j - 13], 7) ^ w[j - 6]) >>> 0;
    }
    for (let j = 0; j < 64; j++) w1[j] = (w[j] ^ w[j + 4]) >>> 0;

    let [a, b, c, d, e, f, g, h] = v;
    for (let j = 0; j < 64; j++) {
      const tj = j < 16 ? 0x79cc4519 : 0x7a879d8a;
      const ss1 = rotl((rotl(a, 12) + e + rotl(tj, j)) >>> 0, 7);
      const ss2 = (ss1 ^ rotl(a, 12)) >>> 0;
      const ff = j < 16 ? a ^ b ^ c : (a & b) | (a & c) | (b & c);
      const gg = j < 16 ? e ^ f ^ g : (e & f) | (~e & g);
      const tt1 = (ff + d + ss2 + w1[j]) >>> 0;
      const tt2 = (gg + h + ss1 + w[j]) >>> 0;
      d = c;
      c = rotl(b, 9);
      b = a;
      a = tt1;
      h = g;
      g = rotl(f, 19);
      f = e;
      e = p0(tt2);
    }
    v[0] = (v[0] ^ a) >>> 0;
    v[1] = (v[1] ^ b) >>> 0;
    v[2] = (v[2] ^ c) >>> 0;
    v[3] = (v[3] ^ d) >>> 0;
    v[4] = (v[4] ^ e) >>> 0;
    v[5] = (v[5] ^ f) >>> 0;
    v[6] = (v[6] ^ g) >>> 0;
    v[7] = (v[7] ^ h) >>> 0;
  }

  const out = new Uint8Array(32);
  const outView = new DataView(out.buffer);
  v.forEach((word, i) => outView.setUint32(i * 4, word));
  return out;
}

/** SM3 hex digest of a UTF-8 string (same output as `sm-crypto` `sm3(str)`). */
export function sm3(message: string): string {
  return Array.from(sm3Bytes(utf8Bytes(message)))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}
