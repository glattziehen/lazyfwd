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

Run `lazyfwd`. The screen has three panels:

1. **Hosts**: your `~/.ssh/config` hosts plus hosts you add. Pinned hosts (`*`) sort first.
2. **Forwards** of the selected host: `●` running, `○` stopped, `✗` failed (the footer shows ssh's error). The laptop end of each forward is a clickable link in terminals that support OSC 8 hyperlinks.
3. **Listening** ports on the selected host, with the owning process. lazyfwd asks the host once with `ssh <host> ss -ltnp` when you enter it, and again only when you press `r`. By default it lists only your own processes, which hides system services such as sshd or tailscaled.

Add a local forward with `a`. The target can be one port, a range, or a list: `4966-4968`, `8080`, `db:5432`. Leave the laptop port empty and lazyfwd picks it: the target port if it is free, otherwise the nearest free port. For example, `4966-4968` with 4966 taken on the laptop gives 4965, 4967, 4968. Set the laptop port to choose it yourself.

Add a remote forward (`ssh -R`, a port on the host that reaches your laptop) with `A`. The remote port defaults to the target port.

Each forward is its own `ssh -N` process, started when you add it. Forwards stop when lazyfwd quits and come back stopped next time. lazyfwd uses `BatchMode=yes`, so hosts must authenticate without a prompt (key or agent).

lazyfwd saves hosts, pins, and forwards in `$XDG_CONFIG_HOME/lazyfwd/workspace.json` (`~/.config` on Linux, `~/Library/Application Support` on macOS).

## Keys

| Key | Panel | Action |
| --- | --- | --- |
| `j` / `k` | any | Move down / up |
| `g` / `G`, `<` / `>` | any | Jump to top / bottom |
| `ctrl+d` / `ctrl+u`, `.` / `,` | any | Move half a page down / up |
| `h` / `l`, `tab`, `1` `2` `3` | any | Switch panel |
| `r` | any | Refresh the host's listening ports |
| `A` | any | Add a remote forward |
| `?` | any | Show or hide the key help |
| `q` | any | Stop all forwards and quit |
| `a` | Hosts | Add a host (any ssh destination, e.g. `user@box`) |
| `p` | Hosts | Pin or unpin the host |
| `d` | Hosts | Remove an added host that has no forwards |
| `enter` | Hosts | Open the host's forwards |
| `a` | Forwards, Listening | Add a local forward |
| `space` | Forwards | Start or stop the forward |
| `e` | Forwards | Edit the forward (restarts it if running) |
| `d` | Forwards | Delete the forward |
| `o` | Forwards | Open the laptop end in the browser |
| `enter` | Listening | Forward this port to the laptop |
| `t` | Listening | Show your ports or all ports |

In forms, `tab` moves between fields, `enter` saves, and `esc` cancels.
