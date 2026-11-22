#!/usr/bin/env bash
# Bootstrap and harden an Ubuntu 24.04 Droplet for booking-platform (QA/PROD, T3.1).
#
# Idempotent: safe to run again. Run as root over SSH:
#   ssh root@HOST 'bash -s' < deploy/server/bootstrap.sh              # step 1: prepare
#   ssh deploy@HOST true                                              # verify key login
#   ssh root@HOST 'bash -s -- --lock-root' < deploy/server/bootstrap.sh  # step 2: lock SSH
#
# Step 1 never disables root login, so a failed step can never lock the operator out.
# See docs/operations/server-bootstrap.md.
set -euo pipefail

LOCK_ROOT=false
[[ "${1:-}" == "--lock-root" ]] && LOCK_ROOT=true

DEPLOY_USER=deploy
SWAP_SIZE_MB=2048
export DEBIAN_FRONTEND=noninteractive

log() { printf '\n==> %s\n' "$*"; }

if [[ $EUID -ne 0 ]]; then
  echo "run as root" >&2
  exit 1
fi
# shellcheck source=/dev/null
. /etc/os-release
if [[ "$ID" != "ubuntu" || "$VERSION_ID" != "24.04" ]]; then
  echo "expected Ubuntu 24.04, found $PRETTY_NAME" >&2
  exit 1
fi

lock_ssh() {
  log "SSH hardening (root login and passwords disabled)"
  if [[ ! -s /home/$DEPLOY_USER/.ssh/authorized_keys ]]; then
    echo "refusing: $DEPLOY_USER has no authorized_keys" >&2
    exit 1
  fi
  cat >/etc/ssh/sshd_config.d/00-booking-hardening.conf <<'EOF'
# Managed by deploy/server/bootstrap.sh
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
AuthenticationMethods publickey
MaxAuthTries 3
LoginGraceTime 30
X11Forwarding no
AllowAgentForwarding no
AllowUsers deploy
EOF
  # Ubuntu cloud images ship 50-cloud-init.conf with PasswordAuthentication; first match wins
  # and 00- sorts first, but validate the effective config before reloading anyway.
  sshd -t
  effective=$(sshd -T)
  grep -qx 'permitrootlogin no' <<<"$effective"
  grep -qx 'passwordauthentication no' <<<"$effective"
  systemctl reload ssh
  log "done: only '$DEPLOY_USER' can log in, with a key"
}

if $LOCK_ROOT; then
  lock_ssh
  exit 0
fi

log "Packages and security updates"
apt-get update -q
apt-get -q -y -o Dpkg::Options::=--force-confold upgrade
apt-get -q -y install ca-certificates curl gnupg ufw fail2ban unattended-upgrades \
  apt-listchanges jq
timedatectl set-timezone UTC

log "Automatic security updates (reboot at 04:30 UTC if required)"
cat >/etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT::Periodic::AutocleanInterval "7";
EOF
cat >/etc/apt/apt.conf.d/52booking-unattended <<'EOF'
Unattended-Upgrade::Automatic-Reboot "true";
Unattended-Upgrade::Automatic-Reboot-Time "04:30";
Unattended-Upgrade::Remove-Unused-Dependencies "true";
EOF

log "Swap (${SWAP_SIZE_MB} MiB)"
if ! swapon --show=NAME --noheadings | grep -qx /swapfile; then
  if [[ ! -f /swapfile ]]; then
    fallocate -l "${SWAP_SIZE_MB}M" /swapfile
    chmod 600 /swapfile
    mkswap /swapfile
  fi
  swapon /swapfile
fi
grep -q '^/swapfile ' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
cat >/etc/sysctl.d/90-booking.conf <<'EOF'
# Prefer RAM; swap only under pressure (PostgreSQL + Argon2 on a small Droplet).
vm.swappiness = 10
vm.vfs_cache_pressure = 50
# Basic network hardening.
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv6.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.tcp_syncookies = 1
EOF
sysctl --system >/dev/null

log "User '$DEPLOY_USER' (key-only, sudo without password, docker)"
if ! id "$DEPLOY_USER" >/dev/null 2>&1; then
  adduser --disabled-password --gecos "" "$DEPLOY_USER"
fi
install -d -m 700 -o "$DEPLOY_USER" -g "$DEPLOY_USER" "/home/$DEPLOY_USER/.ssh"
if [[ -s /root/.ssh/authorized_keys ]]; then
  # Merge root's keys (the operator's) without duplicating entries.
  touch "/home/$DEPLOY_USER/.ssh/authorized_keys"
  sort -u /root/.ssh/authorized_keys "/home/$DEPLOY_USER/.ssh/authorized_keys" \
    -o "/home/$DEPLOY_USER/.ssh/authorized_keys"
fi
chown "$DEPLOY_USER:$DEPLOY_USER" "/home/$DEPLOY_USER/.ssh/authorized_keys"
chmod 600 "/home/$DEPLOY_USER/.ssh/authorized_keys"
echo "$DEPLOY_USER ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/90-$DEPLOY_USER
chmod 440 /etc/sudoers.d/90-$DEPLOY_USER
visudo -cf /etc/sudoers.d/90-$DEPLOY_USER

log "Firewall (ufw): 22, 80, 443 only"
ufw default deny incoming
ufw default allow outgoing
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp # HTTP/3 (Caddy)
ufw --force enable

log "fail2ban (sshd jail)"
cat >/etc/fail2ban/jail.d/booking.local <<'EOF'
[sshd]
enabled = true
backend = systemd
maxretry = 5
findtime = 10m
bantime = 1h
EOF
systemctl enable --now fail2ban
systemctl restart fail2ban

log "Docker Engine + Compose plugin (official repository)"
if ! command -v docker >/dev/null 2>&1; then
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
  chmod a+r /etc/apt/keyrings/docker.asc
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
    >/etc/apt/sources.list.d/docker.list
  apt-get update -q
  apt-get -q -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi
install -d -m 755 /etc/docker
cat >/etc/docker/daemon.json <<'EOF'
{
  "log-driver": "local",
  "log-opts": { "max-size": "10m", "max-file": "5" },
  "live-restore": true,
  "no-new-privileges": true,
  "userland-proxy": false
}
EOF
systemctl enable docker
systemctl restart docker
usermod -aG docker "$DEPLOY_USER"

log "Application directory"
install -d -m 750 -o "$DEPLOY_USER" -g "$DEPLOY_USER" /opt/booking

log "Summary"
echo "os:        $PRETTY_NAME ($(uname -r))"
echo "swap:      $(swapon --show=SIZE --noheadings | head -1)"
echo "docker:    $(docker --version)"
echo "compose:   $(docker compose version --short)"
ufw status | sed -n '1p'
echo "fail2ban:  $(fail2ban-client status sshd | grep 'Currently banned' | xargs)"
echo
echo "Next: verify 'ssh $DEPLOY_USER@<host>' works, then run again with --lock-root."
