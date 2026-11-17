#!/bin/sh
# Renders the terminal UI with made-up hosts to .github/assets/screenshot.svg for the README.
# Linux only. Run it as "pixi run screenshot", which provides go, tmux, and freeze.
#
# The hosts are aliases in a throwaway ssh config that all reach a throwaway sshd on
# 127.0.0.1 (e2e/sshd.sh). The sshd answers every remote command with a fake ss, so the
# Listening panel shows the same made-up ports on every machine. Local forwards need
# no listener behind them to run, so the forwards connect to nothing.
set -eu
repo=$(cd "$(dirname "$0")/.." && pwd)
out=$repo/.github/assets/screenshot.svg
work=$(mktemp -d)
socket=lazyfwd-screenshot
cleanup() {
  tmux -L "$socket" kill-server 2>/dev/null || true
  # ssh masters outlive a lazyfwd that did not quit cleanly.
  pkill -f -- "-F $work/home/.ssh/config" 2>/dev/null || true
  [ -f "$work/sshd/sshd.pid" ] && kill "$(cat "$work/sshd/sshd.pid")" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

mkdir -p "$work/bin"
go build -o "$work/bin/lazyfwd" "$repo"

# The fake ss, and the sshd ForceCommand that puts it first on PATH.
mkdir -p "$work/remote-bin"
cat > "$work/remote-bin/ss" <<'SS'
#!/bin/sh
cat <<'OUT'
State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
LISTEN 0      511        127.0.0.1:8000      0.0.0.0:*    users:(("gunicorn",pid=48121,fd=5))
LISTEN 0      128        127.0.0.1:5432      0.0.0.0:*    users:(("postgres",pid=1207,fd=7))
LISTEN 0      128        127.0.0.1:6006      0.0.0.0:*    users:(("tensorboard",pid=50213,fd=12))
LISTEN 0      128        127.0.0.1:8888      0.0.0.0:*    users:(("jupyter-lab",pid=49870,fd=9))
OUT
SS
cat > "$work/remote-bin/command" <<COMMAND
#!/bin/sh
PATH=$work/remote-bin:\$PATH exec sh -c "\$SSH_ORIGINAL_COMMAND"
COMMAND
chmod +x "$work/remote-bin/ss" "$work/remote-bin/command"

ssh-keygen -q -t ed25519 -N '' -f "$work/id"
port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
"$repo/e2e/sshd.sh" "$work/sshd" "$port" "$work/id.pub"
echo "ForceCommand $work/remote-bin/command" >> "$work/sshd/sshd_config"
kill "$(cat "$work/sshd/sshd.pid")"
sleep 0.2
"$(command -v sshd || echo /usr/sbin/sshd)" -f "$work/sshd/sshd_config" -E "$work/sshd/sshd.log"

# A home whose ssh config names the made-up hosts; lazyfwd lists hosts from it.
mkdir -p "$work/home/.ssh"
cat > "$work/home/.ssh/config" <<CONFIG
Host gpu-box staging db-primary build-runner
  HostName 127.0.0.1
  Port $port
  IdentityFile $work/id
  IdentitiesOnly yes
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  LogLevel ERROR
CONFIG
# ssh reads the real user's ~/.ssh/config whatever HOME says, so a shim points it at
# the throwaway one.
cat > "$work/bin/ssh" <<SSH
#!/bin/sh
exec $(command -v ssh) -F "$work/home/.ssh/config" "\$@"
SSH
chmod +x "$work/bin/ssh"

export HOME="$work/home" XDG_CONFIG_HOME="$work/config" XDG_RUNTIME_DIR="$work/run"
export PATH="$work/bin:$PATH"
mkdir -p "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"

# Local forwards listen on their remote port only when it is free here.
for p in 6006 8000 8888; do
  if ! python3 -c "import socket,sys; socket.socket().bind(('127.0.0.1', int(sys.argv[1])))" "$p" 2>/dev/null; then
    echo "port $p is in use on this machine; free it so the forwards show their remote ports" >&2
    exit 1
  fi
done

lazyfwd host pin gpu-box >/dev/null
lazyfwd forward add gpu-box 8888 6006 >/dev/null
lazyfwd forward add staging 8000 >/dev/null
lazyfwd forward add db-primary 5432 --listen 15432 >/dev/null

cols=120
rows=24
tmux -L "$socket" -f /dev/null new-session -d -x "$cols" -y "$rows" \
  -e HOME="$HOME" -e XDG_CONFIG_HOME="$XDG_CONFIG_HOME" -e XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" \
  -e PATH="$PATH" -e TERM=xterm-256color -e COLORTERM=truecolor lazyfwd
sleep 1
lazyfwd forward start 1 2 3 >/dev/null
# Focus gpu-box's Listening panel, which fetches its ports.
tmux -L "$socket" send-keys 3
sleep 2
tmux -L "$socket" capture-pane -e -p > "$work/screen.ansi"
# Quit, which stops the forwards and closes the ssh connections.
tmux -L "$socket" send-keys q y
sleep 1

# freeze ignores "default foreground" (39) and faint (2): make them a reset and gray.
esc=$(printf '\033')
sed -e "s/$esc\[39m/$esc[0m/g" -e "s/$esc\[\(0;\)\{0,1\}2m/$esc[\190m/g" \
  "$work/screen.ansi" > "$work/screen.fixed"

mkdir -p "$(dirname "$out")"
freeze --language ansi --window --theme dracula --font.size 14 --line-height 1 --padding 20 \
  --border.radius 8 --output "$out" < "$work/screen.fixed"
echo "wrote $out"
