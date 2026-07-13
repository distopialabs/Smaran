#!/usr/bin/env python3
"""Render a kt.py-compatible config by:
1. Reading a static template TOML from configs/<figure>.toml.
2. Substituting @REMOTE_BASE_DIR@, @REMOTE_REPO_DIR@, @REMOTE_BIN_DIR@,
   @REPO_BRANCH@ placeholders from env vars.
3. Prepending a [nodes.node1] / [nodes.node2] section from env vars.
4. Writing the merged TOML to --out.

Env vars (see KeyTransparencyScripts/nodes.env.template for defaults):
  KT_SERVER_HOST, KT_SERVER_PORT, KT_CLIENT_HOST, KT_CLIENT_PORT,
  KT_SSH_USER, KT_SSH_KEY, KT_REMOTE_DIR, KT_REPO_URL, KT_REPO_BRANCH.
"""
from __future__ import annotations

import argparse
import os
from pathlib import Path

DEFAULTS = {
    "KT_SERVER_HOST": "node1",
    "KT_SERVER_PORT": "22",
    "KT_CLIENT_HOST": "node0",
    "KT_CLIENT_PORT": "22",
    "KT_SSH_USER": os.environ.get("USER", ""),
    "KT_SSH_KEY": next(
        (k for k in (os.path.expanduser("~/.ssh/id_cloudlab"),
                     os.path.expanduser("~/.ssh/id_ed25519")) if os.path.exists(k)),
        os.path.expanduser("~/.ssh/id_ed25519"),
    ),
    "KT_REMOTE_DIR": "/users/" + os.environ.get("USER", "shistuu"),
    "KT_REPO_BRANCH": "unified-artifact",
    "KT_REPO_URL": "https://github.com/distopialabs/Smaran",
}


def _env(name: str) -> str:
    return os.environ.get(name, DEFAULTS[name])


def render(template_path: Path, out_path: Path) -> None:
    remote_base = _env("KT_REMOTE_DIR")
    substitutions = {
        "@REMOTE_BASE_DIR@": remote_base,
        "@REMOTE_REPO_DIR@": f"{remote_base}/Smaran",
        "@REMOTE_BIN_DIR@": f"{remote_base}/Smaran/bin",
        "@REPO_BRANCH@": _env("KT_REPO_BRANCH"),
        "@REPO_URL@": _env("KT_REPO_URL"),
    }

    template = template_path.read_text(encoding="utf-8")
    for placeholder, value in substitutions.items():
        template = template.replace(placeholder, value)

    nodes_section = f'''[nodes.node1]
ip = "{_env("KT_SERVER_HOST")}"
port = {int(_env("KT_SERVER_PORT"))}
username = "{_env("KT_SSH_USER")}"
key_path = "{_env("KT_SSH_KEY")}"

[nodes.node2]
ip = "{_env("KT_CLIENT_HOST")}"
port = {int(_env("KT_CLIENT_PORT"))}
username = "{_env("KT_SSH_USER")}"
key_path = "{_env("KT_SSH_KEY")}"

'''
    out_path.write_text(nodes_section + template, encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--template", required=True, type=Path,
                        help="Path to configs/<figure>.toml")
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    render(args.template, args.out)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
