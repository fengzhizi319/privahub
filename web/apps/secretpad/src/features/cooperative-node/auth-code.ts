/**
 * Cooperative-node helpers ported from the legacy
 * `modules/cooperative-node-list/{add-cooperative-node-modal,slectBefore}.tsx`.
 */

export type NetProtocol = 'http://' | 'https://';

/** Parsed node authentication code (base64 of a JSON object). */
export interface ParsedAuthCode {
  certText: string;
  dstNodeId: string;
  name: string;
  /** Address without the protocol prefix. */
  dstNetAddress: string;
  protocol: NetProtocol;
  instId: string;
  instName: string;
  masterNodeId: string;
}

const REQUIRED_KEYS = ['certText', 'dstNodeId', 'name', 'dstNetAddress', 'instId', 'instName', 'masterNodeId'] as const;

/** base64 → UTF-8 string (same as legacy `base64ToBytes`). */
export function decodeBase64Utf8(base64: string): string {
  const bin = atob(base64.trim());
  const bytes = Uint8Array.from(bin, (m) => m.codePointAt(0)!);
  return new TextDecoder().decode(bytes);
}

/**
 * Parse a node authentication code. Legacy rule: the code is base64 JSON and
 * parsing only succeeds when *all* of certText / dstNodeId / name /
 * dstNetAddress / instId / instName / masterNodeId are present.
 * Returns `null` when the code cannot be parsed.
 */
export function parseAuthCode(code: string): ParsedAuthCode | null {
  if (!code || !code.trim()) return null;
  try {
    const obj = JSON.parse(decodeBase64Utf8(code)) as Record<string, unknown>;
    if (!obj || typeof obj !== 'object') return null;
    for (const k of REQUIRED_KEYS) {
      if (!obj[k]) return null;
    }
    const addr = String(obj.dstNetAddress);
    return {
      certText: String(obj.certText),
      dstNodeId: String(obj.dstNodeId),
      name: String(obj.name),
      dstNetAddress: stripProtocol(addr),
      protocol: getProtocol(addr),
      instId: String(obj.instId),
      instName: String(obj.instName),
      masterNodeId: String(obj.masterNodeId),
    };
  } catch {
    return null;
  }
}

/** Encode an auth code (inverse of parseAuthCode; used by tests / tooling). */
export function encodeAuthCode(obj: Record<string, string>): string {
  const bytes = new TextEncoder().encode(JSON.stringify(obj));
  let bin = '';
  bytes.forEach((b) => (bin += String.fromCharCode(b)));
  return btoa(bin);
}

const PROTOCOL_BY_TYPE: Record<string, NetProtocol> = { notls: 'http://', tls: 'https://', mtls: 'https://' };

/** Legacy `getProtocol`: explicit http(s) prefix wins, else derived from the node protocol, else http. */
export function getProtocol(url?: string, protocolType?: string): NetProtocol {
  const fallback = (protocolType && PROTOCOL_BY_TYPE[protocolType.toLowerCase()]) || 'http://';
  if (!url) return fallback;
  const m = url.match(/^(https?):/i);
  return m ? (`${m[1].toLowerCase()}://` as NetProtocol) : fallback;
}

/** Legacy `replaceProtocol`. */
export function stripProtocol(url?: string): string {
  if (!url) return '';
  return url.replace(/(^\w+:|^)\/\//, '');
}

/** host:port (no whitespace, host ≤ 50 chars, port 0–65535) — legacy address rule. */
const NET_ADDRESS_RE =
  /^(?!.*\s)(.{1,50}):([0-9]|[1-9]\d|[1-9]\d{2}|[1-9]\d{3}|[1-5]\d{4}|6[0-4]\d{3}|65[0-4]\d{2}|655[0-2]\d|6553[0-5])$/;

export function isValidNetAddress(address: string): boolean {
  return NET_ADDRESS_RE.test(address);
}
