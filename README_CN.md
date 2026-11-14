# px

px 是个极简 [mihomo](https://github.com/MetaCubeX/mihomo) 控制器。单个二进制包含 TUI、运行内核与 TUN 的守护进程和 systemd 安装器。全部状态保存在一个 JSON 文件中。

[English](README.md) · **简体中文**

## 特性

- **订阅配置**：按链接添加
- **节点切换**：选中的节点写入所有包含它的代理分组
- **TUN**：`px0` 网卡，自动路由加 DNS 劫持，另开混合 HTTP 与 SOCKS 端口
- **流量图表**：守护进程启动即记录吞吐，首页图表显示
- **TUI**：全屏帮助页、带预览的多列浏览器、快捷键可配置、中英双语
- **状态**：一个 JSON 文件保存全部状态

## 构建

```bash
go build -ldflags="-s -w" .
```

## 快速开始

1. 把 `px` 放进 `PATH`。
2. 执行 `sudo px install` 注册并启动 systemd 服务。
3. 执行 `px` 打开 TUI。
4. 按 `n` 添加订阅链接。

## 命令

| 命令 | 含义 |
| --- | --- |
| `px` | 打开 TUI |
| `px run [--config <path>]` | 前台运行守护进程 |
| `px install [--config <path>]` | 注册并启动 systemd 服务 |
| `px uninstall` | 停止并删除 systemd 服务 |
| `-h`、`--help` | 显示帮助页 |
| `-v`、`-V`、`--version` | 显示版本号 |

`run`、`install`、`uninstall` 需要 root，TUI 不需要。

## 配置

全部状态在 `~/.config/px/config.json`，sudo 运行时在 `/root/.config/px/config.json`。`--config` 可指定其他路径。

| 字段 | 默认 | 含义 |
| --- | --- | --- |
| `port` | `17890` | 混合 HTTP 与 SOCKS 监听端口 |
| `delay_url` | `http://www.gstatic.com/generate_204` | 延迟测试目标 |
| `lang` | `en` | 界面语言，`en` 或 `zh` |
| `sort` | `0` | 节点排序，`0` 按地区，`1` 配置顺序 |
| `page` | `0` | TUI 打开时的页面，`0` 首页，`1` 配置页 |
| `keys` | 见下 | 快捷键覆盖 |
| `profiles` | 自动管理 | 守护进程写入的订阅条目 |

改完配置文件重启守护进程生效。

## 快捷键

默认键位如下。每个功能可配多个键，键名支持 `ctrl+` 与 `alt+` 前缀；shift 用换档字符写，shift+a 就写 `A`。

| 键 | 动作 |
| --- | --- |
| `esc`、`q`、`ctrl+c` | 退出，ctrl+c 直接退出不询问 |
| `?` | 帮助页 |
| `tab`、`1`、`2` | 循环或直达页面 |
| `enter`、`space` | 首页开关内核，配置页切换节点 |
| `k` `w` `up` / `j` `s` `down` | 移动 |
| `l` `d` `right` / `h` `a` `left` | 进出配置 |
| `n` / `r` / `x` | 添加、同步、删除配置 |
| `,` | 切换节点排序 |

覆盖示例：

```json
"keys": {
  "global":  {"Quit": ["Q"]},
  "library": {"NavUp": ["u"], "Sort": ["ctrl+s"]},
  "input":   {"CursorLeft": ["alt+b"]}
}
```

动作名：

| 分组 | 动作 |
| --- | --- |
| `global` | `Quit` `ShowHelp` `CyclePages` `SwitchToHome` `SwitchToLibrary` |
| `home` | `ToggleKernel` |
| `library` | `NavUp` `NavDown` `NavEnterDir` `NavExitDir` `SelectNode` `AddProfile` `SyncProfile` `DeleteProfile` `Sort` |
| `input` | `ConfirmInput` `EscapeInput` `InputBackspace` `CursorLeft` `CursorRight` |

## 文件

| 路径 | 职责 |
| --- | --- |
| `config.json` | 只需备份此文件 |
| `profiles/<名字>.yaml` | 下载的订阅正文，同步可重建 |
| `mihomo/` | 内核 geo 数据与缓存，自动重建 |

## 许可证

GPL-3.0
