# Changelog

All notable changes to this project are documented in this file.

## [0.1.0](https://github.com/glattziehen/lazyfwd/releases/tag/v0.1.0) - 2026-11-16

### Features

- Add lazyfwd TUI for ssh port forwards ([#1](https://github.com/glattziehen/lazyfwd/pull/1))
- _(tui)_ Add a Live panel of running and failed forwards across hosts ([#5](https://github.com/glattziehen/lazyfwd/pull/5))
- Share one ssh connection per host and log in on the terminal when ssh prompts ([#8](https://github.com/glattziehen/lazyfwd/pull/8))
- Forward unix sockets, say "local" instead of "laptop" ([#9](https://github.com/glattziehen/lazyfwd/pull/9))
- _(tui)_ Open a forwarded port from the Listening panel, space out host counts ([#10](https://github.com/glattziehen/lazyfwd/pull/10))
- _(tui)_ Esc focuses the Hosts panel ([#11](https://github.com/glattziehen/lazyfwd/pull/11))
- _(tui)_ Show a socket path as a listening end in the add-forward forms ([#19](https://github.com/glattziehen/lazyfwd/pull/19))
- _(tui)_ Move the Live panel to the bottom as [4] ([#20](https://github.com/glattziehen/lazyfwd/pull/20))
- _(tui)_ Ask before quitting, with Yes selected ([#22](https://github.com/glattziehen/lazyfwd/pull/22))

### Bug fixes

- List listening ports on macOS hosts ([#2](https://github.com/glattziehen/lazyfwd/pull/2))
- Portability across themes, platforms, and remote hosts ([#7](https://github.com/glattziehen/lazyfwd/pull/7))
- _(tunnel)_ Fail a forward whose port another program holds on one loopback family ([#12](https://github.com/glattziehen/lazyfwd/pull/12))
- Bugs found by fuzzing: hangs on stalled hosts, duplicate listening ends, refused edits, symlinked state, and more ([#13](https://github.com/glattziehen/lazyfwd/pull/13))
- _(workspace)_ Refuse a workspace.json whose forwards the forms could not make ([#14](https://github.com/glattziehen/lazyfwd/pull/14))
- Trust release tokens from glattziehen/lazyfwd ([#21](https://github.com/glattziehen/lazyfwd/pull/21))
- Print a plain semantic version from VERSION ([#24](https://github.com/glattziehen/lazyfwd/pull/24))

### Documentation

- Shorten README for new users ([#18](https://github.com/glattziehen/lazyfwd/pull/18))

### Build and CI

- Add CI, a changelog-driven release flow, and --version ([#6](https://github.com/glattziehen/lazyfwd/pull/6))
- Lint and format with a lefthook pre-commit hook, also in CI ([#16](https://github.com/glattziehen/lazyfwd/pull/16))
