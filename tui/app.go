package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/timetetng/tqq/media"
	"github.com/timetetng/tqq/napcat"
)

var (
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	chatStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	activeBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("62"))
)

type imageSentMsg struct {
	convIndex int
	filePath  string
}

type Conversation struct {
	ID       int64
	Name     string
	Type     string
	Messages []napcat.Event
	Unread   int
}

type newMessageMsg napcat.Event

type AppModel struct {
	client        *napcat.Client
	selfID        int64
	convs         []Conversation
	current       int
	input         string
	focus         string
	inputMode     bool
	selectedMsg   int
	width         int
	height        int
	showPreview   bool   // 是否开启了图片预览
	previewOutput string // 存放 chafa 渲染出来的 ASCII/Sixel 字符串
	showSidebar   bool   // 为第3点做准备：是否显示侧边栏
	previewIsImg bool
}

func NewApp(client *napcat.Client, selfID int64) AppModel {
	return AppModel{
		client:      client,
		selfID:      selfID,
		focus:       "sidebar",
		showSidebar: true, // 默认开启
	}
}

type fetchHistoryMsg struct{}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		waitForMessage(m.client.Events),
		func() tea.Msg {
			// 初始化时请求最近会话列表
			m.client.CallAPI("get_recent_contact", map[string]interface{}{})
			// 这个 API 会通过 websocket 返回结果，你需要调整 websocket 的 readLoop 
			// 把 API的 response (带有 echo 的 JSON) 也发到 Events 或新的 Channel 中解析
			return nil
		},
	)
}
func waitForMessage(events <-chan napcat.Event) tea.Cmd {
	return func() tea.Msg {
		return newMessageMsg(<-events)
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case newMessageMsg:
		evt := napcat.Event(msg)
		m.upsertMessage(evt)
		return m, waitForMessage(m.client.Events)
	case previewResultMsg:
		if m.showPreview {
			m.previewOutput = msg.content
			m.previewIsImg = msg.isImg
		}
		return m, nil
	case imageSentMsg:
		defer os.Remove(msg.filePath)
		if msg.convIndex < len(m.convs) {
			m.convs[msg.convIndex].Messages = append(m.convs[msg.convIndex].Messages, napcat.Event{
				MessageType: m.convs[msg.convIndex].Type,
				UserID:      m.selfID,
				RawMessage:  "[图片]",
				Message:     []napcat.Segment{{Type: "text", Data: []byte(`{"text":"📷 [已发送图片]"}`)}},
				Sender: napcat.Sender{
					UserID:   m.selfID,
					Nickname: "我",
				},
			})
		}
		return m, nil
	case tea.KeyMsg:
		// 修复1：合并所有的 tea.KeyMsg 处理逻辑
		// 优先处理全局快捷键 1：切换侧边栏
		if msg.String() == "1" && !m.inputMode && !m.showPreview {
			m.showSidebar = !m.showSidebar
			if !m.showSidebar && m.focus == "sidebar" {
				m.focus = "chat" // 侧边栏隐藏后，焦点强制回聊天区
			}
			return m, nil
		}

		// 根据当前焦点分配按键事件
		switch m.focus {
		case "sidebar":
			return m.updateSidebar(msg)
		case "chat":
			return m.updateChat(msg)
		}
	}
	return m, nil
}

func (m AppModel) updateSidebar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.current < len(m.convs)-1 {
			m.current++
		}
	case "k", "up":
		if m.current > 0 {
			m.current--
		}
	case "enter", "l":
		m.focus = "chat"
		if m.current < len(m.convs) {
			m.convs[m.current].Unread = 0
		}
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m AppModel) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showPreview {
		switch msg.String() {
		case "q", "esc", "enter":
			m.showPreview = false
			m.previewOutput = ""
			m.previewIsImg = false
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
		return m, nil // 拦截其他按键
	}
	if m.inputMode {
		switch msg.String() {
		case "esc":
			m.inputMode = false
		case "enter":
			if m.input != "" {
				m.sendText()
				m.input = ""
			}
		case "backspace":
			if len(m.input) > 0 {
				runes := []rune(m.input)
				m.input = string(runes[:len(runes)-1])
			}
		case "ctrl+c":
			return m, tea.Quit
		default:
			if len(msg.Runes) > 0 {
				m.input += string(msg.Runes)
			}
		}
		return m, nil
	}

	// NORMAL 模式
	switch msg.String() {
	case "i":
		m.inputMode = true
	case "esc", "h":
		m.focus = "sidebar"
	case "ctrl+c", "q":
		return m, tea.Quit
	case "j", "down":
		if m.current < len(m.convs) {
			msgs := m.convs[m.current].Messages
			if m.selectedMsg < len(msgs)-1 {
				m.selectedMsg++
			}
		}
	case "k", "up":
		if m.selectedMsg > 0 {
			m.selectedMsg--
		}
	case "enter":
		return m, m.previewSelectedImage()
	case "p":
		return m, m.sendClipboardImage()
	case "f":
		return m, m.sendFileImage()
	}
	return m, nil
}

func (m *AppModel) previewSelectedImage() tea.Cmd {
	if m.current >= len(m.convs) { return nil }
	msgs := m.convs[m.current].Messages
	if m.selectedMsg >= len(msgs) { return nil }
	evt := msgs[m.selectedMsg]

	for _, seg := range evt.Message {
		if seg.Type != "image" { continue }
		var d napcat.ImageData
		json.Unmarshal(seg.Data, &d)
		url := napcat.GetImageURL(d.File, d.URL)
		if url == "" { return nil }
		
		m.showPreview = true
		m.previewOutput = "图片加载中...\n\n(按 'q' 返回)"
		m.previewIsImg = false // 加载中的文字不是图片

		return func() tea.Msg {
			tmp, err := os.CreateTemp("", "tqq-preview-*.jpg")
			if err != nil { return previewResultMsg{"图片下载失败", false} }
			defer os.Remove(tmp.Name())
			tmp.Close()

			if err := exec.Command("curl", "-s", "-L", "-o", tmp.Name(), url).Run(); err != nil {
				return previewResultMsg{"图片下载失败", false}
			}

			// 预先计算右侧面板内部的安全尺寸
			sidebarW := 0
			if m.showSidebar { sidebarW = m.width / 4 }
			chatW := (m.width - sidebarW) / 2 - 2
			previewW := m.width - sidebarW - chatW - 4
			
			w, h := previewW-4, m.height-7
			if w <= 0 { w = 10 }
			if h <= 0 { h = 10 }
			sizeStr := fmt.Sprintf("%dx%d", w, h)

			// 优先尝试 Sixel
			out, err := exec.Command("chafa", "--format", "sixels", "--size", sizeStr, tmp.Name()).Output()
			if err == nil && len(out) > 0 {
				return previewResultMsg{string(out), true} // 成功生成图片
			}
			// 兜底：纯字符画
			out, _ = exec.Command("chafa", "--format", "symbols", "--size", sizeStr, tmp.Name()).Output()
			return previewResultMsg{string(out), false} // 字符画视为普通文本
		}
	}
	return nil
}

type previewResultMsg struct {
	content string
	isImg bool
}

func (m AppModel) sendClipboardImage() tea.Cmd {
	convIndex := m.current
	return func() tea.Msg {
		path, err := media.GetClipboardImage()
		if err != nil || convIndex >= len(m.convs) {
			return nil
		}
		conv := m.convs[convIndex]
		if err := m.client.SendImage(conv.Type, conv.ID, conv.ID, path); err != nil {
			os.Remove(path)
			return nil
		}
		return imageSentMsg{convIndex: convIndex, filePath: path}
	}
}

func (m AppModel) sendFileImage() tea.Cmd {
	convIndex := m.current
	return func() tea.Msg {
		path, err := media.PickImageFile()
		if err != nil || path == "" {
			return nil
		}
		conv := m.convs[convIndex]
		if err := m.client.SendImage(conv.Type, conv.ID, conv.ID, path); err != nil {
			return nil
		}
		return imageSentMsg{convIndex: convIndex, filePath: path}
	}
}

func (m *AppModel) upsertMessage(evt napcat.Event) {
	var id int64
	var name string
	if evt.MessageType == "group" {
		id = evt.GroupID
		name = evt.GroupName
		if name == "" {
			name = fmt.Sprintf("群 %d", evt.GroupID)
		}
	} else {
		id = evt.UserID
		name = evt.Sender.Nickname
	}

	for i := range m.convs {
		if m.convs[i].ID == id {
			m.convs[i].Messages = append(m.convs[i].Messages, evt)
			if i != m.current || m.focus != "chat" {
				m.convs[i].Unread++
			}
			return
		}
	}
	m.convs = append(m.convs, Conversation{
		ID:       id,
		Name:     name,
		Type:     evt.MessageType,
		Messages: []napcat.Event{evt},
		Unread:   1,
	})
}

func (m *AppModel) sendText() {
	conv := m.convs[m.current]
	seg := napcat.Segment{Type: "text"}
	seg.Data = []byte(fmt.Sprintf(`{"text":%q}`, m.input))

	params := napcat.SendMsgParams{
		MessageType: conv.Type,
		Message:     []napcat.Segment{seg},
	}
	if conv.Type == "private" {
		params.UserID = conv.ID
	} else {
		params.GroupID = conv.ID
	}
	m.client.CallAPI("send_msg", params)

	m.convs[m.current].Messages = append(m.convs[m.current].Messages, napcat.Event{
		MessageType: conv.Type,
		UserID:      m.selfID,
		RawMessage:  m.input,
		Message:     []napcat.Segment{seg},
		Sender: napcat.Sender{
			UserID:   m.selfID,
			Nickname: "我",
		},
	})
}

func (m AppModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	
	sidebarW := 0
	if m.showSidebar {
		sidebarW = m.width / 4
	}
	
	chatW := m.width - sidebarW - 4
	previewW := 0
	if m.showPreview {
		chatW = (m.width - sidebarW) / 2 - 2
		previewW = m.width - sidebarW - chatW - 4
	}

	innerH := m.height - 5
	var layout []string

	if m.showSidebar {
		layout = append(layout, m.renderSidebar(sidebarW, innerH))
	}
	layout = append(layout, m.renderChat(chatW, innerH))

	if m.showPreview {
		content := ""
		// 如果是文字或字符画，交给 Lipgloss 去排版
		if !m.previewIsImg {
			content = "\n" + m.previewOutput 
		}

		// 必须恢复固定宽高，这样才能画出一个漂亮的外框作为“底板”
		previewStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Width(previewW).Height(innerH).
			Align(lipgloss.Center) 
		
		layout = append(layout, previewStyle.Render(content))
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top, layout...)
	input := m.renderInput(m.width)

	// 这是底层排版 UI
	ui := lipgloss.JoinVertical(lipgloss.Left, top, input)

	// 【魔法操作】如果存在真正的图片，使用 ANSI 绝对坐标强行画在 UI 上层
	if m.showPreview && m.previewIsImg {
		// 第3行：避开顶部边框 (Row 是 1-indexed)
		row := 3 
		// 计算光标所在的列，加上偏移量避开左侧边框
		col := chatW + 4 
		if m.showSidebar {
			col += sidebarW
		}
		
		// \x1b7: 保存当前光标, \x1b[%d;%dH: 移动光标到绝对位置, \x1b8: 恢复光标
		// 这样既画了图，又不会干扰 Bubbletea 的内部状态
		overlay := fmt.Sprintf("\x1b7\x1b[%d;%dH%s\x1b8", row, col, m.previewOutput)
		ui += overlay
	}

	return ui
}

func (m AppModel) renderSidebar(w, h int) string {
	content := ""
	for i, conv := range m.convs {
		prefix := "  "
		if i == m.current {
			prefix = "> "
		}
		unread := ""
		if conv.Unread > 0 {
			unread = fmt.Sprintf(" [%d]", conv.Unread)
		}
		line := fmt.Sprintf("%s%s%s", prefix, conv.Name, unread)
		if i == m.current {
			line = titleStyle.Render(line)
		}
		content += line + "\n"
	}
	if content == "" {
		content = "暂无会话\n等待消息..."
	}
	style := sidebarStyle
	if m.focus == "sidebar" {
		style = activeBorder
	}
	return style.Width(w).Height(h).Render(content)
}

func (m AppModel) renderChat(w, h int) string {
	if m.current >= len(m.convs) {
		style := chatStyle
		if m.focus == "chat" {
			style = activeBorder
		}
		return style.Width(w).Height(h).Render("← 选择一个会话")
	}

	conv := m.convs[m.current]
	innerW := w - 4
	lines := []string{}

	for i, evt := range conv.Messages {
		name := evt.Sender.Card
		if name == "" {
			name = evt.Sender.Nickname
		}
		rendered := napcat.RenderMessage(evt.Message)
		isSelected := (i == m.selectedMsg)

		var line string
		if evt.UserID == m.selfID {
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Width(innerW).
				Align(lipgloss.Right)
			if isSelected {
				style = style.Background(lipgloss.Color("22"))
			}
			line = style.Render("[我] " + rendered)
		} else {
			nameStr := lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Render(name + ": ")
			msgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
			if isSelected {
				msgStyle = msgStyle.Background(lipgloss.Color("17"))
			}
			line = nameStr + msgStyle.Render(rendered)
		}
		lines = append(lines, line)
	}

	if len(lines) > h-2 {
		lines = lines[len(lines)-(h-2):]
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}

	style := chatStyle
	if m.focus == "chat" {
		style = activeBorder
	}
	return style.Width(w).Height(h).Render(content)
}

func (m AppModel) renderInput(w int) string {
	var modeTag, prompt, cursor string

	if m.focus != "chat" {
		modeTag = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render(" SIDEBAR ")
		prompt = "  "
		cursor = ""
	} else if m.inputMode {
		modeTag = lipgloss.NewStyle().
			Background(lipgloss.Color("149")).
			Foreground(lipgloss.Color("0")).
			Bold(true).
			Render(" INSERT ")
		prompt = "> "
		cursor = "█"
	} else {
		modeTag = lipgloss.NewStyle().
			Background(lipgloss.Color("86")).
			Foreground(lipgloss.Color("0")).
			Bold(true).
			Render(" NORMAL ")
		prompt = "  "
		cursor = "▌"
	}

	inputArea := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(w - 4).
		Render(prompt + m.input + cursor)

	return lipgloss.JoinHorizontal(lipgloss.Center, modeTag, " ", inputArea)
}
