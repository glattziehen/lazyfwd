#!/bin/sh
# Starts the sshd that the end-to-end test runs against, on 127.0.0.1:PORT
# with its own host key, accepting only the public key PUBKEY. It runs on
# Linux, on macOS, and in WSL for the Windows job.
#
#   e2e/sshd.sh DIR PORT PUBKEY
set -eu
dir=$1
port=$2
pubkey=$3

mkdir -p "$dir"
rm -f "$dir/host_key" "$dir/host_key.pub"
ssh-keygen -q -t ed25519 -N '' -f "$dir/host_key"
# A key file written on Windows ends its line with CRLF.
tr -d '\r' < "$pubkey" > "$dir/authorized_keys"
cat > "$dir/sshd_config" <<CONFIG
ListenAddress 127.0.0.1
Port $port
HostKey $dir/host_key
AuthorizedKeysFile $dir/authorized_keys
PasswordAuthentication no
KbdInteractiveAuthentication no
StrictModes no
PidFile $dir/sshd.pid
AllowTcpForwarding yes
AllowStreamLocalForwarding yes
CONFIG
# sshd moves to the background once it listens.
"$(command -v sshd || echo /usr/sbin/sshd)" -f "$dir/sshd_config" -E "$dir/sshd.log"
