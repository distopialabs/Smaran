#!/usr/bin/env bash
# CloudLab profile startup — runs as root on every node at instantiation
# (and on reboots; every step is idempotent). Zero reviewer action: when this
# finishes, the node is ready to run experiments.
#
#   Usage: setup-node.sh <server|client> <server-lan-ip>
#
# Progress/diagnostics: /local/setup.log (the profile redirects our output
# there). SSH logins get an accurate READY / IN PROGRESS / FAILED banner —
# CloudLab's green "ready" fires at boot, before this script finishes.
set -euo pipefail

ROLE="${1:?usage: setup-node.sh <server|client> <server-lan-ip>}"
SERVER_IP="${2:?usage: setup-node.sh <server|client> <server-lan-ip>}"
REPO=/local/repository
STATUS_FILE=/local/setup.status

echo "IN PROGRESS" >"$STATUS_FILE"
trap 'echo FAILED >"$STATUS_FILE"' ERR

die() {
    echo "ERROR: $*"
    echo FAILED >"$STATUS_FILE"
    exit 1
}

# --- MOTD readiness banner (before anything that can fail) -------------------
cat >/etc/update-motd.d/05-smaran-artifact <<'MOTD'
#!/bin/sh
status=$(cat /local/setup.status 2>/dev/null || echo UNKNOWN)
case "$status" in
    READY)         echo "Smaran artifact: node setup READY — start at /local/repository/README.md" ;;
    "IN PROGRESS") echo "Smaran artifact: node setup IN PROGRESS (~2 min) — watch: tail -f /local/setup.log" ;;
    *)             echo "Smaran artifact: node setup FAILED — see /local/setup.log" ;;
esac
MOTD
chmod +x /etc/update-motd.d/05-smaran-artifact

# --- The dataset must be populated ---------------------------------------------
# Under the profile the dataset is always mounted, so missing content is
# always an error - fail loudly here rather than confusingly at run time.
# (A dataset still attached read-write to another experiment presents its
# pre-release state to read-only mounts: often an empty filesystem.)
for f in modified_accounts account_stats_50k.csv account_stats_all.csv \
         smaran-paper-logs.tar.gz; do
    [ -e "/smaran-dataset/$f" ] || die "/smaran-dataset/$f missing - dataset not populated yet (or still attached read-write to another experiment)"
done

# --- Who instantiated the experiment (the reviewer) --------------------------
CREATOR="$(geni-get user_urn | awk -F'+' '{print $NF}')"
CREATOR_HOME="$(getent passwd "$CREATOR" | cut -d: -f6)"
[ -n "$CREATOR_HOME" ] || { echo "cannot resolve home of $CREATOR"; exit 1; }

# --- Cluster config consumed by DecentralizedLedgerScripts/lib/common.sh -----
# On the client, SERVER_HOST points at the server's experiment-LAN address;
# the server itself (and anyone logged into it) runs in localhost mode.
HOST_FOR_SCRIPTS="$SERVER_IP"
[ "$ROLE" = "server" ] && HOST_FOR_SCRIPTS=localhost
cat >/local/cluster.env <<EOF
SERVER_HOST=$HOST_FOR_SCRIPTS
SMARAN_DATASET_DIR=/smaran-dataset
EOF

# --- Intra-experiment SSH (client drives the server over ssh) ----------------
# geni-get returns the experiment creator's CloudLab key; homes are
# NFS-shared across the experiment's nodes, so installing it once under
# ~/.ssh makes node0 -> node1 ssh work without prompts.
SSH_DIR="$CREATOR_HOME/.ssh"
KEY="$SSH_DIR/id_cloudlab"
mkdir -p "$SSH_DIR"
if [ ! -f "$KEY" ]; then
    geni-get key >"$KEY"
    chmod 600 "$KEY"
    ssh-keygen -y -f "$KEY" >"$KEY.pub"
    chown "$CREATOR:" "$KEY" "$KEY.pub"
fi
touch "$SSH_DIR/authorized_keys"
grep -qF -f "$KEY.pub" "$SSH_DIR/authorized_keys" || cat "$KEY.pub" >>"$SSH_DIR/authorized_keys"
if [ ! -f "$SSH_DIR/config" ] || ! grep -q "id_cloudlab" "$SSH_DIR/config"; then
    cat >>"$SSH_DIR/config" <<EOF
Host 192.168.1.* node0 node1
    IdentityFile $KEY
    StrictHostKeyChecking accept-new
EOF
    chmod 600 "$SSH_DIR/config"
    chown "$CREATOR:" "$SSH_DIR/config"
fi

# --- Writable working areas ---------------------------------------------------
# Server: /data is the NVMe blockstore (ingested DBs). Client: plain dir on
# the OS disk (paper-logs staging only — small).
mkdir -p /data/local
chown "$CREATOR:" /data/local
# The repo clone must be writable by the reviewer (results/, bin/, logs).
chown -R "$CREATOR:" "$REPO"

# --- Toolchain + build + dataset links (idempotent; near-no-op on the baked
# image, ~5-10 min of apt/pip on a stock Ubuntu image) ------------------------
sudo -u "$CREATOR" -i bash -c "cd '$REPO' && ./DecentralizedLedgerScripts/install_smaran.sh"

# --- Client: pre-extract the paper-logs bundle so Tier 0 starts instantly ----
if [ "$ROLE" = "client" ] && [ ! -f /data/local/artifact-staging/paper-logs/MANIFEST.txt ]; then
    mkdir -p /data/local/artifact-staging
    tar -xzf /smaran-dataset/smaran-paper-logs.tar.gz -C /data/local/artifact-staging
    chown -R "$CREATOR:" /data/local/artifact-staging
fi

echo READY >"$STATUS_FILE"
echo "setup-node.sh: $ROLE node ready"
