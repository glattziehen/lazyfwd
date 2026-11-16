# lazyfwd

Manage SSH port forwards from a keyboard-first terminal UI, in the spirit of lazygit. Pick a host, see its listening ports, and forward them without remembering SSH flags. You can also open a forwarded web port in your browser.

lazyfwd uses your system `ssh`, so your SSH config, agent, and ProxyJump work as usual. It runs on Linux, macOS, WSL, and Windows, where some things work differently: see [Windows](#windows).

- Pick hosts from your SSH config and pin the ones you use most.
- See your processes' listening ports and process names when available, then forward a port with one key.
- See, stop, and open running forwards across hosts in the Live panel. Web ports have clickable local links in supported terminals.
- Forward TCP ports and Unix sockets in either direction (`-L` or `-R`).
- Keep the same local port as the remote port by default, or use the nearest free one. For example, `4966-4968` stays `4966-4968` when free.

<!-- Screenshot goes here. -->

## Install

You need `ssh` on your `PATH`.

**Release binary:** Download the archive for your system from the [releases page](https://github.com/glattziehen/lazyfwd/releases), check it against `checksums.txt`, and put `lazyfwd` on your `PATH`. If Gatekeeper blocks a browser download on macOS, run `xattr -d com.apple.quarantine lazyfwd`.

**Go:** With Go 1.24 or later, run:

```sh
go install github.com/glattziehen/lazyfwd@latest
```

**From source:** Clone the repository and build with Go 1.24 or later:

```sh
git clone https://github.com/glattziehen/lazyfwd.git
cd lazyfwd
go build .
```

This writes `./lazyfwd`. To enable the contributor hooks, run `pixi run lefthook-install` once.

## Use

Run `lazyfwd`. Select a host from your SSH config with `j` and `k`, then press `enter`. If your host is not listed, press `a` in the Hosts panel to add it.

Use `tab` to switch panels. In Listening, select a port and press `space` to forward it to your machine. In Forwards, press `a` to add a local forward by hand, or `space` to start or stop one. Press `A` to add a remote forward, or `o` to open a forward's local end in your browser. Press `?` for all keys.

Forwards stop when you quit lazyfwd and return stopped the next time you run it. If SSH needs a password, host key confirmation, or another login step, lazyfwd hands the terminal to SSH and returns after it connects.

Hosts, pins, and forwards are saved in `$XDG_CONFIG_HOME/lazyfwd/workspace.json` (`~/.config` on Linux, `~/Library/Application Support` on macOS, `%AppData%` on Windows).

## Command line

Scripts and agents can do everything the UI does with commands. They go through the running `lazyfwd`, so the UI shows their changes at once. Without the UI, run `lazyfwd serve` to hold the connections:

```sh
lazyfwd serve &
lazyfwd forward add box 4966-4968 --start   # remote ports 4966-4968 to the same local ports
lazyfwd forward list --json
```

Run `lazyfwd --help` for the commands, their JSON output, and exit codes.

## Windows

lazyfwd uses the ssh that Windows ships, which cannot share one connection per host. So each forward runs its own ssh, and lazyfwd refuses three kinds of forwards, with the reason:

- Forwards to hosts that ask for a password, a key passphrase, or 2FA on every connect. Use a key that ssh can use without asking, such as one in `ssh-agent`.
- Forwards to hosts whose SSH config has `LocalForward`, `RemoteForward`, or `DynamicForward` lines.
- Unix sockets as a forward's local end. Sockets on the remote host work.

When a host's key is unknown, lazyfwd hands the terminal to ssh so you can accept the key, as on Linux and macOS.

CI tests lazyfwd on Windows Server 2025 against a Linux SSH server. The tests cover forwards in both directions, sockets on the remote host, the command line and `lazyfwd serve`, and that no ssh is left after lazyfwd quits or crashes. The terminal UI in a real console, opening a browser, and the host key login are not tested on Windows.
