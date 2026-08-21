#!/usr/bin/env bash
# YTsaurus / tractoai node golden: private Ubuntu + Docker + sysctl.
# Does not install ytserver-all (no public apt repo on Evolution).
set -euxo pipefail

export DEBIAN_FRONTEND=noninteractive
export NEEDRESTART_MODE=a

cloud-init status --wait || true

# Public Ubuntu metadata points apt at mirror.yandex.ru. Guest DNS often
# cannot resolve it. Ubuntu 24.04 uses deb822 (ubuntu.sources), not sources.list.
rewrite_mirrors() {
  local f
  for f in /etc/apt/sources.list /etc/apt/sources.list.d/ubuntu.sources /etc/apt/sources.list.d/*.list; do
    [[ -f "$f" ]] || continue
    sed -i 's|http://mirror.yandex.ru/ubuntu|http://archive.ubuntu.com/ubuntu|g' "$f" || true
  done
}
rewrite_mirrors

# cloud-init / unattended-upgrades hold dpkg after first boot.
wait_apt() {
  local i
  for i in $(seq 1 90); do
    if apt-get -o DPkg::Lock::Timeout=30 -o Acquire::Retries=3 update -y; then
      return 0
    fi
    sleep 5
  done
  return 1
}
wait_apt

apt-get -o DPkg::Lock::Timeout=300 install -y --no-install-recommends \
  ca-certificates \
  curl \
  jq \
  python3 \
  chrony \
  docker.io \
  containerd

systemctl enable docker chrony
systemctl start docker || true

swapoff -a || true
if [[ -f /etc/fstab ]]; then
  sed -i.bak '/\sswap\s/s/^/#/' /etc/fstab || true
fi

cat >/etc/sysctl.d/99-ytsaurus.conf <<'EOF'
vm.max_map_count = 262144
fs.file-max = 1048576
fs.inotify.max_user_instances = 1024
fs.inotify.max_user_watches = 1048576
net.core.somaxconn = 4096
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_tw_reuse = 1
EOF
sysctl --system || true

install -d -m 0755 /etc/ytsaurus
cat >/etc/ytsaurus/image-release <<EOF
name=ytsaurus-ubuntu-24-04
baked_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
docker=$(docker --version 2>/dev/null || echo missing)
EOF

apt-get clean
rm -rf /var/lib/apt/lists/*

echo "ytsaurus golden provision done"
