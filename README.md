# lazyfwd

A keyboard-first TUI for ssh port forwards, in the spirit of lazygit. Pick a host, forward ports in either direction, and click the laptop end to open it in the browser.

lazyfwd runs your system `ssh`, so `~/.ssh/config`, agents, and ProxyJump work as usual. It reads hosts from `~/.ssh/config` (including `Include`d files) and never edits it.

## Install

With Go 1.24 or later:

```sh
go install github.com/moritzwilksch/lazyfwd@latest
```

Without Go, use [pixi](https://pixi.sh) from a checkout:

```sh
pixi run build   # writes ./lazyfwd
```

## Usage

Run `lazyfwd`. The screen has four panels, numbered by their key:

0. **Live** forwards of all hosts, across the top: every running or failed forward, so you can start, stop, open, or edit it without picking its host first. A stopped forward leaves the list; a failed one stays, with ssh's error in the footer, until you restart or delete it. `enter` shows the forward in its host's Forwards panel.
1. **Hosts**: your `~/.ssh/config` hosts plus hosts you add. Pinned hosts (`*`) sort first. `●2 ✗1` counts a host's running and failed forwards.
2. **Forwards** of the selected host: `●` running, `○` stopped, `✗` failed (the footer shows ssh's error). Each row reads laptop end, arrow, remote end; `→` is a local forward and `←` a remote one. The laptop end is a clickable link in terminals that support OSC 8 hyperlinks.
3. **Listening** ports on the selected host, with the owning process. A forwarded port shows the forward's status and laptop end, e.g. `● → localhost:4966`. lazyfwd asks the host over one ssh command when you enter it, and again only when you press `r`: `ss -ltnp` on Linux, `lsof` and `netstat` on macOS. By default it lists only your own processes, which hides system services such as sshd or tailscaled; `t` shows all ports, with `-` in place of the process name for other users' services.

Colours come from your terminal's 16-colour palette, so your theme applies: green is running, red is failed, grey is stopped or a hint, cyan marks the focused panel, blue a clickable laptop end, and yellow a pin.

Add a local forward with `a`. The target can be one port, a range, or a list: `4966-4968`, `8080`, `db:5432`. Leave the laptop port empty and lazyfwd picks it: the target port if it is free, otherwise the nearest free port. For example, `4966-4968` with 4966 taken on the laptop gives 4965, 4967, 4968. Set the laptop port to choose it yourself.

Add a remote forward (`ssh -R`, a port on the host that reaches your laptop) with `A`. The remote port defaults to the target port.

Forwards start when you add them, stop when lazyfwd quits, and come back stopped next time.

## Connections and logins

lazyfwd opens one ssh connection per host, an OpenSSH ControlMaster, and puts all of that host's forwards and the Listening fetch on it. It first connects without prompts. A host that authenticates with a key or agent connects with no extra step.

When ssh needs you, lazyfwd hands the terminal to a plain `ssh` for that host and returns once it connects. This covers a first connection to an unknown host key, a password, a key passphrase without an agent, and 2FA. One login serves every forward and fetch on that host until lazyfwd quits or the connection drops. A login starts only after you start a forward or enter a host, never on its own. If the login fails, ssh's messages stay on screen until you press enter; `ctrl+c` at a prompt cancels it. Start the forward again to retry.

If a connection drops (network loss, host restart, or about 45 seconds without an answer), its forwards turn failed with the reason in the footer. Starting one opens a new connection, with a login if needed.

lazyfwd passes its own `ControlMaster`, `ControlPath`, and `ControlPersist`, so these settings in `~/.ssh/config` do not apply to its connections, and lazyfwd never uses or closes your own masters. It keeps its sockets in `$XDG_RUNTIME_DIR/lazyfwd-<uid>`, else in the temp directory, else in `/tmp/lazyfwd-<uid>`. If lazyfwd dies without quitting (`kill -9`, a closed terminal), its connections keep running and hold their ports until the next start of lazyfwd closes them.

Limits:

- A host key that changed fails like ssh does: lazyfwd shows ssh's warning and never edits `known_hosts`. Fix the entry yourself.
- A host that refuses your key and offers no password fails the login at once; lazyfwd shows ssh's message.
- lazyfwd needs OpenSSH with ControlMaster support, so it runs on Linux and macOS, not Windows.

lazyfwd saves hosts, pins, and forwards in `$XDG_CONFIG_HOME/lazyfwd/workspace.json` (`~/.config` on Linux, `~/Library/Application Support` on macOS).

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
