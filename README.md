# lazyfwd

A keyboard-first TUI for ssh port forwards, in the spirit of lazygit. Pick a host, forward ports in either direction, and click the local end to open it in the browser.

lazyfwd runs your system `ssh`, so `~/.ssh/config`, agents, and ProxyJump work as usual. It lists the hosts named in `Host` lines of `~/.ssh/config` and `/etc/ssh/ssh_config` (including `Include`d files) and never edits them. Hosts that only a `Match` block or a pattern such as `Host *.lab` describes are not listed; add them with `a`.

## Platforms

lazyfwd runs on Linux and macOS; in WSL it runs as on Linux. Windows itself is not supported for now.

The remote host can be Linux (with `ss`, `netstat`, or only `/proc`), macOS, or a BSD. lazyfwd lists a remote host's listening ports with whichever of `ss`, `netstat`, `sockstat`, `lsof`, or `/proc/net/tcp` the host has. On a host without a POSIX `sh`, such as Windows, the Listening panel says it cannot list ports; forwards you add by hand work everywhere.

<!-- Screenshot goes here. -->

## Install

lazyfwd runs on Linux and macOS and needs `ssh` on your `PATH`.

**Release binary.** Download the archive for your system from the [releases page](https://github.com/moritzwilksch/lazyfwd/releases), check it against `checksums.txt`, and put `lazyfwd` on your `PATH`:

```sh
tar -xzf lazyfwd_*_darwin_arm64.tar.gz lazyfwd
sudo mv lazyfwd /usr/local/bin/
```

On macOS, a browser download is quarantined; run `xattr -d com.apple.quarantine lazyfwd` if Gatekeeper blocks it.

**go install.** With Go 1.24 or later:

```sh
go install github.com/moritzwilksch/lazyfwd@latest
```

**From source.** In a checkout, run `go build .` with Go 1.24 or later, or `pixi run build` with [pixi](https://pixi.sh), which brings its own Go. Both write `./lazyfwd`.

`lazyfwd --version` prints the version.

## Usage

Run `lazyfwd`. The screen has four panels, numbered by their key:

0. **Live** forwards of all hosts, across the top: every running or failed forward, so you can start, stop, open, or edit it without picking its host first. A stopped forward leaves the list; a failed one stays, with ssh's error in the footer, until you restart or delete it. `enter` shows the forward in its host's Forwards panel.
1. **Hosts**: the hosts of your ssh config plus hosts you add. Pinned hosts (`*`) sort first. `● 2  ✗ 1` counts a host's running and failed forwards.
2. **Forwards** of the selected host: `●` running, `○` stopped, `✗` failed (the footer shows ssh's error). Each row reads local end, arrow, remote end; `→` is a local forward and `←` a remote one. The local end is a clickable link in terminals that support OSC 8 hyperlinks: `https://` when the target port is 443 or 8443, `http://` otherwise, and no link for well-known non-web ports such as 22, 3306, 5432, or 6379.
3. **Listening** ports on the selected host, with the owning process. A forwarded port shows the forward's status and local end, e.g. `● → localhost:4966`. lazyfwd asks the host over one ssh command when you enter it, and again only when you press `r`. By default it lists only your own processes, which hides system services such as sshd or tailscaled and Docker's published ports, which belong to root; the panel's last line counts the hidden ports, and `t` shows all ports, with `-` in place of the process name for other users' services. On a Linux host with neither `ss` nor `netstat`, no port has a process name, so only `t` shows them.

Colours come from your terminal's 16-colour palette, so your theme applies: green is running, red is failed, faint text is stopped or a hint, cyan marks the focused panel, blue a clickable local end, and yellow a pin.

Every forward has a local end, on this machine, and a remote end, on the host. A local forward (`ssh -L`) listens on its local end and connects to its remote end; a remote forward (`ssh -R`) listens on its remote end and connects to its local end.

Add a local forward with `a`. The remote end can be one port, a range, or a list: `4966-4968`, `8080`, `db:5432`. Leave the local end empty and lazyfwd picks a port: the target port if it is free, otherwise the nearest free port. For example, `4966-4968` with 4966 taken on the local machine gives 4965, 4967, 4968. Set the local end to choose the port yourself.

Add a remote forward with `A`. Its remote end defaults to the target port.

`e` edits a forward. The form shows a port on localhost as the port alone, so changing it means replacing one word.

Forwards start when you add them, stop when lazyfwd quits, and come back stopped next time.

## Unix sockets

Either end of a forward can be a unix socket instead of a TCP port, in both directions. Type the socket's path where a form asks for an end, for example:

- local forward, remote end `/var/run/docker.sock`, local end `2375`: `docker -H tcp://localhost:2375` reaches the host's Docker.
- local forward, remote end `8080`, local end `/tmp/app.sock`: a local socket that reaches port 8080 on the host.
- remote forward, local end `/run/user/1000/app.sock`, remote end `/tmp/app.sock`: a socket on the host that reaches a local one.

A socket path must be absolute and may hold only letters, digits, and `/ . _ + @ % = -`, at most 103 bytes. ssh expands neither `~` nor relative paths on either end, so lazyfwd refuses them. When the target is a socket, there is no port to copy, so set the listening end yourself.

A socket end shows its path. Only a TCP local end is a link, and `o` on a socket end says why it cannot open.

ssh leaves a forward's socket file behind when the forward stops, and refuses to listen on an existing file. When lazyfwd starts a forward on a socket path, it therefore removes a socket there that nothing accepts on: on the local machine itself, and on the host after `ssh -W` finds the socket dead. It never removes a socket that accepts connections or a file that is not a socket; the forward then fails with the reason. Socket files stay behind after lazyfwd quits until the forward next starts.

The Listening panel shows TCP ports only.

## Connections and logins

lazyfwd opens one ssh connection per host, an OpenSSH ControlMaster, and puts all of that host's forwards and the Listening fetch on it. It first connects without prompts. A host that authenticates with a key or agent connects with no extra step.

When ssh needs you, lazyfwd hands the terminal to a plain `ssh` for that host and returns once it connects. This covers a first connection to an unknown host key, a password, a key passphrase without an agent, and 2FA. One login serves every forward and fetch on that host until lazyfwd quits or the connection drops. A login starts only after you start a forward or enter a host, never on its own. If the login fails, ssh's messages stay on screen until you press enter; `ctrl+c` at a prompt cancels it. Start the forward again to retry.

If a connection drops (network loss, host restart, or about 45 seconds without an answer), its forwards turn failed with the reason in the footer. Starting one opens a new connection, with a login if needed.

lazyfwd passes its own `ControlMaster`, `ControlPath`, and `ControlPersist`, so these settings in `~/.ssh/config` do not apply to its connections, and lazyfwd never uses or closes your own masters. Its connections also skip the config's `LocalForward`, `RemoteForward`, and `DynamicForward` lines, so only the forwards lazyfwd shows hold ports. It keeps its sockets in `$XDG_RUNTIME_DIR/lazyfwd-<uid>`, else in the temp directory, else in `/tmp/lazyfwd-<uid>`. If lazyfwd dies without quitting (`kill -9`, a closed terminal), its connections keep running and hold their ports until the next start of lazyfwd closes them.

Limits:

- A host key that changed fails like ssh does: lazyfwd shows ssh's warning and never edits `known_hosts`. Fix the entry yourself.
- A host that refuses your key and offers no password fails the login at once; lazyfwd shows ssh's message.
- lazyfwd needs OpenSSH with ControlMaster support, so it runs on Linux and macOS, not Windows.

`o` opens the local end with `open` on macOS, and with `wslview` in WSL or `xdg-open` on Linux. On a machine without a graphical session, such as a server you reached over ssh, it names the URL to open yourself.

lazyfwd saves hosts, pins, and forwards in `$XDG_CONFIG_HOME/lazyfwd/workspace.json` (`~/.config` on Linux, `~/Library/Application Support` on macOS). Only one lazyfwd runs per workspace file; a second one exits with a message.

## Keys

| Key | Panel | Action |
| --- | --- | --- |
| `j` / `k` | any | Move down / up |
| `g` / `G`, `<` / `>` | any | Jump to top / bottom |
| `ctrl+d` / `ctrl+u`, `.` / `,` | any | Move half a page down / up |
| `h` / `l`, `tab`, `0` `1` `2` `3` | any | Switch panel |
| `r` | any | Refresh the host's listening ports |
| `A` | any | Add a remote forward |
| `?` | any | Show or hide the key help |
| `q` | any | Stop all forwards and quit |
| `space`, `s` | Live | Start or stop the forward |
| `enter` | Live | Show the forward in its host's Forwards panel |
| `e` | Live | Edit the forward (restarts it if running) |
| `d` | Live | Delete the forward |
| `o` | Live | Open the local end in the browser |
| `a` | Hosts | Add a host (any ssh destination, e.g. `user@box`) |
| `p` | Hosts | Pin or unpin the host |
| `d` | Hosts | Remove an added host that has no forwards |
| `enter` | Hosts | Open the host's forwards |
| `a` | Forwards, Listening | Add a local forward |
| `space`, `s` | Forwards | Start or stop the forward |
| `e` | Forwards | Edit the forward (restarts it if running) |
| `d` | Forwards | Delete the forward |
| `o` | Forwards | Open the local end in the browser |
| `space`, `enter`, `f` | Listening | Forward this port to the local machine, or start or stop its forward |
| `o` | Listening | Open the port's forward in the browser |
| `t` | Listening | Show your ports or all ports |

In forms, `tab` moves between fields, `enter` saves, and `esc` cancels.
