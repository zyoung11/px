# px

px is a minimal [mihomo](https://github.com/MetaCubeX/mihomo) controller. One binary contains the TUI, the daemon that runs the kernel and TUN, and the systemd installer. All state is stored in one JSON file.

**English** · [简体中文](README_CN.md)

## Features

- **Profiles**: add subscriptions by link
- **TUN**: `px0` with auto route and DNS hijack, plus a mixed HTTP and SOCKS port
- **Node switching**: a picked node is set on every proxy group that contains it
- **Traffic charts**: throughput recorded from startup, shown on the Home page
- **TUI**: full screen help, multi column browser with preview, configurable keys, English and Chinese
- **State**: one JSON file stores all state

## Build

```bash
go build -ldflags="-s -w" .
```

## Quick Start

1. Put `px` somewhere on your `PATH`.
2. Run `sudo px install` to register and start the systemd service.
3. Run `px` to open the TUI.
4. Press `n` to add a subscription link.

## Commands

| Command | Meaning |
| --- | --- |
| `px` | Open the TUI |
| `px run [--config <path>]` | Run the daemon in the foreground |
| `px install [--config <path>]` | Register and start the systemd service |
| `px uninstall` | Stop and remove the systemd service |
| `-h`, `--help` | Show the help page |
| `-v`, `-V`, `--version` | Show the version |

`run`, `install` and `uninstall` require root. The TUI does not.

## Configuration

Everything lives in `~/.config/px/config.json` or `/root/.config/px/config.json` when the daemon runs under sudo. `--config` picks another path.

| Field | Default | Meaning |
| --- | --- | --- |
| `port` | `17890` | Mixed HTTP and SOCKS listener port |
| `delay_url` | `http://www.gstatic.com/generate_204` | Latency test target |
| `chart_width` | `50` | Traffic chart width in cells, 20 to 200 |
| `lang` | `en` | UI language, `en` or `zh` |
| `sort` | `0` | Node sort order, `0` region and `1` config order |
| `align` | `false` | Aligned node columns |
| `page` | `0` | Page the TUI opens on, `0` Home and `1` Profiles |
| `keys` | see below | Keybinding overrides |
| `profiles` | managed | Subscription entries written by the daemon |

Restart the daemon after editing the file.

## Keybindings

Defaults below. Every action takes a list of keys and each key may carry `ctrl+` and `alt+` prefixes; write shift as the shifted character, so `shift+a` is `A`.

| Keys | Action |
| --- | --- |
| `esc`, `q`, `ctrl+c` | Quit, ctrl+c quits without asking |
| `?` | Help page |
| `tab`, `1`, `2` | Cycle or jump to a page |
| `enter`, `space` | Toggle kernel on Home, switch node in Profiles |
| `k` `w` `up` / `j` `s` `down` | Move |
| `l` `d` `right` / `h` `a` `left` | Enter or leave a profile |
| `n` / `r` / `x` | Add, sync or delete a profile |
| `,` | Toggle the node sort order |
| `t` | Test the delay of every node in the profile |
| `.` | Toggle the aligned node columns |

Override example:

```json
"keys": {
  "global":  {"Quit": ["Q"]},
  "library": {"NavUp": ["u"], "Sort": ["ctrl+s"]},
  "input":   {"CursorLeft": ["alt+b"]}
}
```

Action names:

| Section | Actions |
| --- | --- |
| `global` | `Quit` `ShowHelp` `CyclePages` `SwitchToHome` `SwitchToLibrary` |
| `home` | `ToggleKernel` |
| `library` | `NavUp` `NavDown` `NavEnterDir` `NavExitDir` `SelectNode` `AddProfile` `SyncProfile` `DeleteProfile` `Sort` `Delay` `Align` |
| `input` | `ConfirmInput` `EscapeInput` `InputBackspace` `CursorLeft` `CursorRight` |

## Files

| Path | Role |
| --- | --- |
| `config.json` | Back up this file only |
| `profiles/<name>.yaml` | Downloaded subscription bodies, rebuilt by sync |
| `mihomo/` | Kernel geodata and caches, rebuilt automatically |

## License

GPL-3.0
