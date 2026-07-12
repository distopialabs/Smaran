# -*- coding: utf-8 -*-
"""Smaran artifact evaluation - two-node experiment (client + server).

This profile reproduces the paper's benchmark topology:

  * **node0 (client)** - where you log in. Runs the orchestration scripts,
    the proof clients (32 concurrent), and the plotting; every file you look
    at (logs, figure PDFs) lands under `/local/repository/results/` here.
  * **node1 (server)** - runs ingestion and the protocol servers against
    NVMe-backed databases. The scripts drive it over SSH; you never need to
    log into it.

Both nodes mount the SmaranEthereumDataset (blocks + account CSVs + curated
paper logs) read-only at `/smaran-dataset`, and the repository is cloned to
`/local/repository` on each node. Node setup is fully automatic (~4 min after
boot): wait until the Startup column on the experiment page shows
**Finished** for both nodes, then SSH into node0 and follow
`/local/repository/README.md`. If you SSH in early, the login banner shows
setup progress.

Default node types are the paper's pair (r6615 server + c6420 client at
Clemson). If they are unavailable, pick a fallback from the parameter lists -
but keep server and client in the same cluster, because the dataset is
cluster-local (Clemson types, or the Utah pair c6525-100g + xl170).
"""

import base64

import geni.portal as portal
import geni.rspec.pg as pg

# Which cluster each allowed hardware type belongs to. The dataset is
# cluster-local, so server and client must come from the same cluster.
CLUSTER_OF = {
    "r6615": "clemson",
    "r650": "clemson",
    "r6525": "clemson",
    "c6420": "clemson",
    "c6525-100g": "utah",
    "xl170": "utah",
}

DATASET_URN = {
    "clemson": "urn:publicid:IDN+clemson.cloudlab.us:distopialabs-pg0+ltdataset+SmaranEthereumDataset",
    "utah": "urn:publicid:IDN+utah.cloudlab.us:distopialabs-pg0+ltdataset+SmaranEthereumDatasetUth",
}

# Per-cluster disk image with the toolchain baked in (Go, LaTeX, python
# plotting stack; see cloudlab/IMAGE.md). The startup script installs
# anything missing, so a stock Ubuntu 22.04 image also works - just slower
# on first boot.
# TODO(image): replace with the per-cluster snapshots once taken (IMAGE.md).
IMAGE = {
    "clemson": "urn:publicid:IDN+emulab.net+image+emulab-ops//UBUNTU22-64-STD",
    "utah": "urn:publicid:IDN+emulab.net+image+emulab-ops//UBUNTU22-64-STD",
}

# The artifact repository, shallow-cloned to /local/repository by the
# startup command. This profile is maintained as profile.py in that repo and
# pasted into the CloudLab profile editor (not a repo-based profile - the
# repo's git history exceeds CloudLab's 500 MiB clone limit), so the clone
# is ours to do. At submission time, flip REPO_REF to the artifact tag.
REPO_URL = "https://github.com/distopialabs/Smaran.git"
REPO_REF = "timing_debug"

# Experiment-LAN addresses; setup-node.sh writes the server's address into
# /local/cluster.env so the scripts find it (benchmark traffic crosses the
# experiment LAN, as in the paper - not the control network).
SERVER_IP = "192.168.1.1"
CLIENT_IP = "192.168.1.2"
NETMASK = "255.255.255.0"

pc = portal.Context()

pc.defineParameter(
    "server_type", "Server node type (ingestion + protocol servers; needs NVMe)",
    portal.ParameterType.STRING, "r6615",
    [
        ("r6615", "r6615 - paper's server: 32-core EPYC 9354P, 192GB, 800GB NVMe (Clemson)"),
        ("r650", "r650 - fallback: 2x36-core Xeon 8360Y, 256GB, 1.6TB NVMe (Clemson)"),
        ("r6525", "r6525 - fallback: 2x32-core EPYC 7543, 256GB, 1.6TB NVMe (Clemson)"),
        ("c6525-100g", "c6525-100g - Utah: 24-core EPYC 7402P, 128GB, 1.6TB NVMe"),
    ],
    longDescription="The server ingests blocks and serves proofs from "
    "~1000 shard databases; SATA/HDD storage is painfully slow for this, so "
    "only NVMe-equipped types are offered. Prefer the paper's r6615.")

pc.defineParameter(
    "client_type", "Client node type (orchestration + proof clients; any ~16+ cores)",
    portal.ParameterType.STRING, "c6420",
    [
        ("c6420", "c6420 - paper's client: 2x16-core Xeon Gold 6142, 384GB (Clemson)"),
        ("r650", "r650 (Clemson)"),
        ("r6525", "r6525 (Clemson)"),
        ("xl170", "xl170 - Utah: 10-core Xeon E5-2640v4, 64GB"),
    ],
    longDescription="The client is mostly idle between benchmark points; it "
    "needs only moderate cores for the 32 concurrent proof clients. Must be "
    "in the same cluster as the server.")

pc.defineParameter(
    "autoSetup", "Run automatic node setup at startup",
    portal.ParameterType.BOOLEAN, True,
    longDescription="Leave checked for the standard artifact-evaluation "
    "flow: nodes clone the repository, install the toolchain, build, and "
    "wire everything up with no manual steps. Uncheck to get bare nodes "
    "(dataset mounted, LAN and blockstore configured, nothing installed); "
    "the SSH login banner then shows the two commands that perform the "
    "setup manually.")

params = pc.bindParameters()

server_cluster = CLUSTER_OF[params.server_type]
client_cluster = CLUSTER_OF[params.client_type]
if server_cluster != client_cluster:
    pc.reportError(portal.ParameterError(
        "Server and client must be in the same cluster (the dataset is "
        "cluster-local): pick Clemson types for both, or the Utah pair "
        "c6525-100g + xl170.", ["server_type", "client_type"]))
pc.verifyParameters()

request = pc.makeRequestRSpec()


def add_node(name, hwtype, role, lan_ip):
    node = request.RawPC(name)
    node.hardware_type = hwtype
    node.disk_image = IMAGE[server_cluster]

    # Experiment-LAN interface (benchmark traffic).
    lan_iface = node.addInterface("if" + name)
    lan_iface.addAddress(pg.IPv4Address(lan_ip, NETMASK))

    # Read-only mount of the cluster's Smaran dataset.
    fsnode = request.RemoteBlockstore("ds" + name, "/smaran-dataset")
    fsnode.dataset = DATASET_URN[server_cluster]
    fsnode.readonly = True
    fslink = request.Link("dslink" + name)
    fslink.addInterface(node.addInterface("ifds" + name))
    fslink.addInterface(fsnode.interface)
    fslink.best_effort = True
    fslink.vlan_tagging = True
    fslink.link_multiplexing = True

    # Startup. Auto-setup on (default): shallow-clone the repo (the ref's
    # tree only, a few MB), then hand off to its setup script - zero
    # reviewer action (~4 min). Idempotent across reboots: the clone is
    # skipped when already present. Auto-setup off: leave the node bare and
    # install only an MOTD breadcrumb with the manual setup commands (it is
    # overwritten by the real READY/FAILED banner once setup runs).
    if params.autoSetup:
        cmd = ("sudo bash -c '{ { [ -d /local/repository/.git ] || "
               "git clone --depth 1 --branch %s %s /local/repository; } && "
               "bash /local/repository/cloudlab/setup-node.sh %s %s; } "
               ">/local/setup.log 2>&1'" % (REPO_REF, REPO_URL, role, SERVER_IP))
    else:
        motd = ("#!/bin/sh\n"
                'echo "Smaran artifact: auto-setup was disabled at instantiation."\n'
                'echo "To set up this node:"\n'
                'echo "  git clone --depth 1 --branch %s %s /local/repository"\n'
                'echo "  sudo /local/repository/cloudlab/setup-node.sh"\n'
                % (REPO_REF, REPO_URL))
        cmd = ("sudo bash -c 'echo %s | base64 -d "
               ">/etc/update-motd.d/05-smaran-artifact && "
               "chmod +x /etc/update-motd.d/05-smaran-artifact'"
               % base64.b64encode(motd.encode()).decode())
    node.addService(pg.Execute(shell="bash", command=cmd))
    return node, lan_iface


server, server_lan = add_node("node1", params.server_type, "server", SERVER_IP)
client, client_lan = add_node("node0", params.client_type, "client", CLIENT_IP)

# Ingested databases live on the server's NVMe blockstore, mounted at /data
# (the scripts use /data/local/artifact-dbs, created by setup-node.sh).
bs = server.Blockstore("bsnode1", "/data")
bs.size = "700GB"

# Client <-> server experiment LAN. All three flags matter for mapping:
# best_effort (no guaranteed cross-rack bandwidth - the paper pair lives in
# different rack groups), and vlan_tagging + link_multiplexing so this LAN
# can share a physical port with the dataset link (the r6615 has a single
# experiment interface). Best-effort still runs at line rate in practice,
# and the benchmark's proof traffic is far below 10G.
link = request.Link("lan0")
link.best_effort = True
link.vlan_tagging = True
link.link_multiplexing = True
link.addInterface(client_lan)
link.addInterface(server_lan)

pc.printRequestRSpec(request)
