/**
 * json_data for inst/node/register. Java (`InstRegisterRequest`, see
 * deploy/common/utils.sh post_kuscia_node) expects
 * `{domainId, token, mode, port, protocol, transPort}`; the Go backend reads
 * `{nodeId, nodeName, netAddress, ...}` — both key sets are sent.
 */
export interface InstRegisterFields {
  domainId: string;
  nodeName?: string;
  token?: string;
  mode: string;
  port: string;
  protocol: string;
  transPort: string;
  netAddress?: string;
}

export function buildRegisterJson(f: InstRegisterFields): string {
  const obj: Record<string, string> = {
    domainId: f.domainId.trim(),
    nodeId: f.domainId.trim(),
    token: (f.token || '').trim(),
    mode: f.mode,
    port: f.port.trim(),
    protocol: f.protocol,
    transPort: f.transPort.trim(),
  };
  if (f.nodeName?.trim()) obj.nodeName = f.nodeName.trim();
  if (f.netAddress?.trim()) obj.netAddress = f.netAddress.trim();
  return JSON.stringify(obj);
}

/** Validate a raw json_data string: must be a JSON object with domainId (or nodeId). */
export function validateRegisterJson(raw: string): string | null {
  try {
    const o = JSON.parse(raw) as Record<string, unknown>;
    if (!o || typeof o !== 'object' || Array.isArray(o)) return 'object';
    if (!o.domainId && !o.nodeId) return 'domainId';
    return null;
  } catch {
    return 'json';
  }
}
