# lazyfwd

Manage SSH port forwards from a keyboard-first terminal UI, in the spirit of lazygit. Pick a host, see its listening ports, and forward them without remembering SSH flags. You can also open a forwarded web port in your browser.

lazyfwd uses your system `ssh`, so your SSH config, agent, and ProxyJump work as usual. It runs on Linux and macOS, including WSL, but not Windows itself.

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
