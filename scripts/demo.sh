#!/bin/sh
# Records the terminal UI with made-up hosts to .github/assets/demo-{dark,light}.gif for
# the README, and the same recordings as .mp4 next to them.
# Linux and macOS. Run it as "pixi run demo", which provides go and vhs with its ttyd and ffmpeg,
# and the Geist Mono font the tape sets.
# scripts/demo.tape holds the keystrokes.
#
# The hosts are aliases in a throwaway ssh config that all reach a throwaway sshd on
# 127.0.0.1 (e2e/sshd.sh). The sshd answers every remote command with a fake uname that
# says Linux and a fake ss, so the Listening panel shows the same made-up ports on every
# machine, macOS included. Local forwards need
# no listener behind them to run, so the forwards connect to nothing.
set -eu
repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
cleanup() {
  # ssh masters outlive a lazyfwd that did not quit cleanly.
  pkill -f -- "-F $work/home/.ssh/config" 2>/dev/null || true
  [ -f "$work/sshd/sshd.pid" ] && kill "$(cat "$work/sshd/sshd.pid")" 2>/dev/null || true
  # Only the font files this script added to ~/Library/Fonts.
  [ -f "$work/added-fonts" ] && while IFS= read -r f; do rm -f "$f"; done < "$work/added-fonts"
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

# vhs draws the terminal in headless Chromium, which silently falls back to another font when
# the tape's font is missing. Linux Chromium finds fonts through fontconfig, so a config
# adds pixi's font-ttf-geist to the system fonts. macOS Chromium only reads the system and
# user font folders, so the font goes into ~/Library/Fonts for the recording.
fonts=${CONDA_PREFIX:?run this as pixi run demo}/fonts
if [ "$(uname -s)" = Darwin ]; then
  : > "$work/added-fonts"
  for f in "$fonts"/GeistMono-*.ttf; do
    dest="$HOME/Library/Fonts/$(basename "$f")"
    [ -e "$dest" ] && continue
    cp "$f" "$dest"
    echo "$dest" >> "$work/added-fonts"
  done
else
  cat > "$work/fonts.conf" <<FONTS
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">
<fontconfig>
  <include ignore_missing="yes">/etc/fonts/fonts.conf</include>
  <dir>$fonts</dir>
  <cachedir>$work/fontconfig-cache</cachedir>
</fontconfig>
FONTS
  export FONTCONFIG_FILE="$work/fonts.conf"
fi
if [ -z "$(fc-list 'Geist Mono' family)" ]; then
  echo "Geist Mono is not visible to fontconfig; the recording would use a fallback font" >&2
  exit 1
fi

mkdir -p "$work/bin"
go build -o "$work/bin/lazyfwd" "$repo"

# The fake uname and ss, and the sshd ForceCommand that puts them first on PATH.
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
cat > "$work/remote-bin/uname" <<UNAME
#!/bin/sh
[ "\$*" = -s ] && echo Linux && exit
exec $(command -v uname) "\$@"
UNAME
cat > "$work/remote-bin/command" <<COMMAND
#!/bin/sh
PATH=$work/remote-bin:\$PATH exec sh -c "\$SSH_ORIGINAL_COMMAND"
COMMAND
chmod +x "$work/remote-bin/ss" "$work/remote-bin/uname" "$work/remote-bin/command"

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

export LAZYFWD_DEMO_HOME="$work/home" XDG_RUNTIME_DIR="$work/run"
export PATH="$work/bin:$PATH"
mkdir -p "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"

# Local forwards listen on their remote port only when it is free here.
for p in 8000 8888; do
  if ! python3 -c "import socket,sys; socket.socket().bind(('127.0.0.1', int(sys.argv[1])))" "$p" 2>/dev/null; then
    echo "port $p is in use on this machine; free it so the forwards show their remote ports" >&2
    exit 1
  fi
done

# record THEME OUTPUT records the tape in THEME to OUTPUT.gif and OUTPUT.mp4, each time
# from the same fresh workspace.
record() {
  export XDG_CONFIG_HOME="$work/config-$1"
  # macOS ignores XDG_CONFIG_HOME and keeps the workspace under HOME.
  rm -rf "$LAZYFWD_DEMO_HOME/Library"
  # lazyfwd lists hosts from $HOME/.ssh/config. The tape switches HOME inside its shell
  # only, so vhs keeps its browser download in the real home.
  HOME=$LAZYFWD_DEMO_HOME lazyfwd host pin gpu-box >/dev/null
  HOME=$LAZYFWD_DEMO_HOME lazyfwd forward add staging 8000 >/dev/null
  HOME=$LAZYFWD_DEMO_HOME lazyfwd forward add db-primary 5432 --listen 15432 >/dev/null
  printf 'Output %s.gif\nOutput %s.mp4\nSet Theme "%s"\n' "$2" "$2" "$1" |
    cat - "$repo/scripts/demo.tape" > "$work/demo.tape"
  # Chromium's sandbox needs user namespaces, which Ubuntu restricts.
  (cd "$repo" && VHS_NO_SANDBOX=true vhs "$work/demo.tape")
}
record Dracula .github/assets/demo-dark
record "Catppuccin Latte" .github/assets/demo-light
