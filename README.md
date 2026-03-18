# TQQ (Terminal QQ)

TQQ 是一个使用 Go 语言和 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建的、基于终端的轻量 QQ 客户端。它以 [NapCatQQ](https://github.com/NapNeko/NapCatQQ) 作为后端，支持远程部署，本地只需要运行 **30MB** 左右的 TUI 客户端。
## ✨ 主要特性

图像支持、剪贴板/文件发送图片; 纯TUI，即使是 tty 也能运行。

## 🛠️ 环境要求与依赖

为了获得完整的体验，请确保您的系统中安装了以下依赖：

* **Go**: 1.26.1 或更高版本
* **NapCatQQ**: 需要作为后台 API 服务运行。

(可选)

* **chafa**: 用于在终端中渲染图片预览。
* **wl-clipboard** (`wl-paste`): 用于在 Wayland 环境下发送剪贴板中的图片。
* **zenity**: 用于弹出图形化文件选择框来选择发送图片。


## 🚀 快速开始

```bash
go install github.com/timetetng/tqq@latest

# 运行
tqq
```

## ⚙️ 配置说明

在首次启动前或启动后，您需要在 `~/.config/tqq/config.toml` 中配置以下信息：

```toml
[napcat]
# NapCatQQ 的 WebSocket URL (用于接收事件和调用 API)
ws_url = "ws://localhost:3001"
# NapCatQQ 的 HTTP URL (用于获取图片等)
http_url = "http://localhost:3000"
# 访问令牌 Token 
token = "your_access_token_here"
# 你自己的 QQ 号
self_id = 123456789

[ui]
# Sixel 渲染时的图片宽度 (px)
image_width = 200
# 是否显示头像 (目前代码中预留的配置项)
show_avatar = true

```

## ⌨️ 快捷键指南

操作分为“Normal（普通）模式”和“Insert（插入）模式”。

### 全局 / Normal 模式

* `1` : 切换显示/隐藏侧边栏（会话列表）
* `q` / `Ctrl+c` : 退出程序

### 侧边栏 (Sidebar) 焦点时

* `j` / `↓` : 选择下一个会话
* `k` / `↑` : 选择上一个会话
* `Enter` / `l` : 进入选中的会话聊天框（并清除未读标记）

### 聊天框 (Chat) 焦点时

* `j` / `↓` : 选中下一条消息
* `k` / `↑` : 选中上一条消息
* `i` : 进入 **Insert 模式**（输入消息）
* `Esc` / `h` : 将焦点移回侧边栏
* `Enter` : 如果当前选中的消息包含图片，则打开图片预览（在预览界面按 `q` 或 `Esc` 返回）
* `p` : 发送剪贴板中的图片（需要 `wl-paste`）
* `f` : 从文件管理器选择图片发送（需要 `zenity`或其他文件管理器）

### Insert 模式 (输入框)

* `Enter` : 发送输入的文本消息
* `Esc` : 取消输入，退回到 Normal 模式
* `Backspace` : 删除字符
