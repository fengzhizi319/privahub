#!/usr/bin/env python3
# Copyright 2026 privahub authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Dump SecretFlow component definitions for privahub.

Produces (relative to --out, default ``config``):

* ``components/secretflow.json`` - the SecretFlow ``CompListDef`` (JSON, proto
  field names in camelCase exactly like Java SecretPad's config/components).
* ``i18n/secretflow.json`` - the SecretFlow translation archive (``gettext``).

Run inside an environment where the local ``secretflow`` package imports, e.g.::

    conda activate sf310
    PYTHONPATH=/path/to/secretflow python scripts/gen_components.py --out config
"""
import argparse
import json
import os
import sys


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default="config")
    args = parser.parse_args()

    from google.protobuf import json_format

    from secretflow.component.core import load_plugins
    from secretflow.component.core.i18n import get_translation, gettext
    from secretflow.component.core.utils import get_comp_list_def

    load_plugins()
    comp_list = get_comp_list_def()
    comp_json = json_format.MessageToDict(comp_list, preserving_proto_field_name=False)

    comp_dir = os.path.join(args.out, "components")
    i18n_dir = os.path.join(args.out, "i18n")
    os.makedirs(comp_dir, exist_ok=True)
    os.makedirs(i18n_dir, exist_ok=True)

    with open(os.path.join(comp_dir, "secretflow.json"), "w", encoding="utf-8") as f:
        json.dump(comp_json, f, ensure_ascii=False, indent=2)

    try:
        archives = get_translation()
    except Exception as e:  # pragma: no cover - translation resources optional
        print(f"warn: load translation failed: {e}", file=sys.stderr)
        archives = {}
    text = gettext(comp_list, archives, None, build_root=True)
    with open(os.path.join(i18n_dir, "secretflow.json"), "w", encoding="utf-8") as f:
        json.dump(text, f, ensure_ascii=False, indent=2)

    print(f"dumped {len(comp_list.comps)} components to {comp_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
