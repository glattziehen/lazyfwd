# lazyfwd

A keyboard-first TUI for ssh port forwards, in the spirit of lazygit. Pick a host, forward ports in either direction, and click the laptop end to open it in the browser.

lazyfwd runs your system `ssh`, so `~/.ssh/config`, agents, and ProxyJump work as usual. It lists the hosts named in `Host` lines of `~/.ssh/config` and `/etc/ssh/ssh_config` (including `Include`d files) and never edits them. Hosts that only a `Match` block or a pattern such as `Host *.lab` describes are not listed; add them with `a`.

## Platforms

lazyfwd supports Linux and macOS laptops; in WSL it runs as on Linux. Windows itself is not supported for now.

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
1. **Hosts**: the hosts of your ssh config plus hosts you add. Pinned hosts (`*`) sort first. `●2 ✗1` counts a host's running and failed forwards.
2. **Forwards** of the selected host: `●` running, `○` stopped, `✗` failed (the footer shows ssh's error). Each row reads laptop end, arrow, remote end; `→` is a local forward and `←` a remote one. The laptop end is a clickable link in terminals that support OSC 8 hyperlinks: `https://` when the target port is 443 or 8443, `http://` otherwise, and no link for well-known non-web ports such as 22, 3306, 5432, or 6379.
3. **Listening** ports on the selected host, with the owning process. A forwarded port shows the forward's status and laptop end, e.g. `● → localhost:4966`. lazyfwd asks the host over one ssh command when you enter it, and again only when you press `r`. By default it lists only your own processes, which hides system services such as sshd or tailscaled and Docker's published ports, which belong to root; the panel's last line counts the hidden ports, and `t` shows all ports, with `-` in place of the process name for other users' services. On a Linux host with neither `ss` nor `netstat`, no port has a process name, so only `t` shows them.

Colours come from your terminal's 16-colour palette, so your theme applies: green is running, red is failed, faint text is stopped or a hint, cyan marks the focused panel, blue a clickable laptop end, and yellow a pin.

Add a local forward with `a`. The target can be one port, a range, or a list: `4966-4968`, `8080`, `db:5432`. Leave the laptop port empty and lazyfwd picks it: the target port if it is free, otherwise the nearest free port. For example, `4966-4968` with 4966 taken on the laptop gives 4965, 4967, 4968. Set the laptop port to choose it yourself.

Add a remote forward (`ssh -R`, a port on the host that reaches your laptop) with `A`. The remote port defaults to the target port.

Each forward is its own `ssh -N` process, started when you add it. Forwards stop when lazyfwd quits and come back stopped next time. lazyfwd uses `BatchMode=yes`, so hosts must authenticate without a prompt (key or agent).

`o` opens the laptop end with `open` on macOS, and with `wslview` in WSL or `xdg-open` on Linux. On a machine without a graphical session, such as a server you reached over ssh, it names the URL to open yourself.

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
| `o` | Live | Open the laptop end in the browser |
| `a` | Hosts | Add a host (any ssh destination, e.g. `user@box`) |
| `p` | Hosts | Pin or unpin the host |
| `d` | Hosts | Remove an added host that has no forwards |
| `enter` | Hosts | Open the host's forwards |
| `a` | Forwards, Listening | Add a local forward |
| `space`, `s` | Forwards | Start or stop the forward |
| `e` | Forwards | Edit the forward (restarts it if running) |
| `d` | Forwards | Delete the forward |
| `o` | Forwards | Open the laptop end in the browser |
| `space`, `enter`, `f` | Listening | Forward this port to the laptop, or start or stop its forward |
| `t` | Listening | Show your ports or all ports |

In forms, `tab` moves between fields, `enter` saves, and `esc` cancels.
