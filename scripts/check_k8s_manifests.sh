#!/usr/bin/env bash
# =============================================================================
# check_k8s_manifests.sh — deployment manifests must keep the hardened baseline.
#
# Kubernetes (deployments/k8s/**.yaml):
#   * YAML parses, no duplicate keys (a duplicate silently overrides the first);
#   * every workload pod: runAsNonRoot, seccompProfile RuntimeDefault, and
#     every container: readOnlyRootFilesystem=true,
#     allowPrivilegeEscalation=false, capabilities.drop [ALL], not privileged;
#   * automountServiceAccountToken=false, no hostNetwork/hostPID/hostIPC/hostPath;
#   * images carry an explicit version (no ':latest', no bare name);
#   * env vars that look like secrets use valueFrom, never literal values;
#   * the Namespace enforces Pod Security "restricted";
#   * a default-deny NetworkPolicy (Ingress+Egress, empty podSelector) exists.
# Compose (deployments/**/docker-compose*.yml, plus extra files as args):
#   * sensitive ports (9001 inner, 8082/8083 KusciaAPI, 8070/8071 DataMesh,
#     8092, 9091) must be published on 127.0.0.1, never 0.0.0.0/implicit;
#   * no ':latest' images for our own (privahub) images, and no ':latest'
#     defaults in ${VAR:-image:latest}.
# =============================================================================
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib/preflight.sh
source scripts/lib/preflight.sh
require_python_module yaml PyYAML || exit 1

echo "Deployment manifest gate"
python3 - "$@" <<'PY'
import glob, re, sys, yaml

class UniqueKeyLoader(yaml.SafeLoader):
    pass
def construct_mapping(loader, node, deep=False):
    seen = set()
    for k, _ in node.value:
        key = loader.construct_object(k, deep=deep)
        if key in seen:
            raise yaml.constructor.ConstructorError(None, None, f"duplicate key {key!r}", k.start_mark)
        seen.add(key)
    return loader.construct_mapping(node, deep)
UniqueKeyLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, construct_mapping)

errs = []
def err(f, msg): errs.append(f"{f}: {msg}")

k8s_files = sorted(glob.glob("deployments/k8s/**/*.yaml", recursive=True) + glob.glob("deployments/k8s/**/*.yml", recursive=True))
compose_files = sorted(set(glob.glob("deployments/**/docker-compose*.y*ml", recursive=True) + sys.argv[1:]))
if not k8s_files:
    print("  [FAIL] no Kubernetes manifests under deployments/k8s - gate would pass vacuously"); sys.exit(1)

SECRETISH = re.compile(r'(SECRET|TOKEN|PASSWORD|PASSWD|_KEY$|AES|PRIVATE|CREDENTIAL)', re.I)
docs = []
for f in k8s_files:
    try:
        for d in yaml.load_all(open(f, encoding="utf-8"), Loader=UniqueKeyLoader):
            if d:
                docs.append((f, d))
    except yaml.YAMLError as e:
        err(f, f"YAML error: {e}")

def image_ok(img):
    if '@sha256:' in img: return True
    last = img.split('/')[-1]
    return ':' in last and not last.endswith(':latest')

workloads = 0
has_psa = has_deny = False
for f, d in docs:
    kind = d.get("kind"); md = d.get("metadata", {}) or {}; name = md.get("name")
    if kind == "Namespace":
        if (md.get("labels") or {}).get("pod-security.kubernetes.io/enforce") != "restricted":
            err(f, f"Namespace {name}: pod-security.kubernetes.io/enforce must be 'restricted'")
        else:
            has_psa = True
    if kind == "NetworkPolicy":
        sp = d.get("spec", {})
        if sp.get("podSelector") in ({}, None) and set(sp.get("policyTypes", [])) == {"Ingress", "Egress"} \
                and not sp.get("ingress") and not sp.get("egress"):
            has_deny = True
    if kind == "Secret" and not f.endswith(".example.yaml"):
        err(f, f"Secret {name}: real Secret manifests must not be committed (use *.example.yaml + out-of-band creation)")
    pod = None
    if kind in ("Deployment", "StatefulSet", "DaemonSet", "Job", "ReplicaSet"):
        pod = d["spec"]["template"]["spec"]
    elif kind == "CronJob":
        pod = d["spec"]["jobTemplate"]["spec"]["template"]["spec"]
    elif kind == "Pod":
        pod = d["spec"]
    if pod is None:
        continue
    workloads += 1
    w = f"{kind}/{name}"
    psc = pod.get("securityContext") or {}
    if pod.get("automountServiceAccountToken") is not False:
        err(f, f"{w}: automountServiceAccountToken must be false")
    for h in ("hostNetwork", "hostPID", "hostIPC"):
        if pod.get(h):
            err(f, f"{w}: {h} is forbidden")
    for v in pod.get("volumes") or []:
        if "hostPath" in v:
            err(f, f"{w}: hostPath volume {v.get('name')} is forbidden")
    for c in (pod.get("initContainers") or []) + (pod.get("containers") or []):
        cn = f"{w}[{c.get('name')}]"
        sc = c.get("securityContext") or {}
        get = lambda k: sc.get(k, psc.get(k))
        if get("runAsNonRoot") is not True: err(f, f"{cn}: runAsNonRoot must be true")
        if (get("runAsUser") or 1) == 0: err(f, f"{cn}: runAsUser must not be 0")
        if sc.get("readOnlyRootFilesystem") is not True: err(f, f"{cn}: readOnlyRootFilesystem must be true")
        if sc.get("allowPrivilegeEscalation") is not False: err(f, f"{cn}: allowPrivilegeEscalation must be false")
        if sc.get("privileged"): err(f, f"{cn}: privileged is forbidden")
        drops = [x.upper() for x in ((sc.get("capabilities") or {}).get("drop") or [])]
        if "ALL" not in drops: err(f, f"{cn}: capabilities.drop must include ALL")
        if (sc.get("capabilities") or {}).get("add"): err(f, f"{cn}: capabilities.add is forbidden")
        sp = (sc.get("seccompProfile") or psc.get("seccompProfile") or {}).get("type")
        if sp != "RuntimeDefault" and sp != "Localhost": err(f, f"{cn}: seccompProfile.type must be RuntimeDefault")
        if not image_ok(c.get("image", "")): err(f, f"{cn}: image {c.get('image')!r} must be version-pinned (no :latest)")
        for e in c.get("env") or []:
            if SECRETISH.search(e.get("name", "")) and "value" in e and e["value"] not in ("", None):
                err(f, f"{cn}: env {e['name']} has a literal value - use valueFrom.secretKeyRef")
if workloads == 0: err("deployments/k8s", "no workload found")
if not has_psa: err("deployments/k8s", "no Namespace with pod-security enforce=restricted")
if not has_deny: err("deployments/k8s", "no default-deny NetworkPolicy (podSelector {} + Ingress+Egress, no rules)")

SENSITIVE = {"9001", "8082", "8083", "8070", "8071", "8092", "9091", "8060", "8061"}
for f in compose_files:
    try:
        d = yaml.load(open(f, encoding="utf-8"), Loader=UniqueKeyLoader) or {}
    except (yaml.YAMLError, FileNotFoundError) as e:
        err(f, f"compose YAML error: {e}"); continue
    for sname, svc in (d.get("services") or {}).items():
        img = str(svc.get("image", ""))
        if re.search(r':latest\b', img) or re.search(r':-[^}]*:latest', img):
            err(f, f"service {sname}: image {img!r} uses :latest")
        for p in svc.get("ports") or []:
            p = str(p) if not isinstance(p, dict) else f"{p.get('host_ip','')}:{p.get('published','')}:{p.get('target','')}"
            parts = re.sub(r'\$\{[^}]*\}', 'VAR', p).split(":")
            container = parts[-1].split("/")[0]
            host_ip = parts[0] if len(parts) == 3 else ""
            if container in SENSITIVE and host_ip not in ("127.0.0.1", "::1", "[::1]"):
                err(f, f"service {sname}: port {p!r} publishes sensitive port {container} beyond loopback (use 127.0.0.1:...)")

if errs:
    print(f"  [FAIL] {len(errs)} manifest violation(s):")
    for e in errs: print(f"      {e}")
    sys.exit(1)
print(f"  [OK]   {len(k8s_files)} k8s manifest(s), {workloads} workload(s), {len(compose_files)} compose file(s): hardened baseline holds")
PY
