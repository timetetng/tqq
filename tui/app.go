package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/timetetng/tqq/media"
	"github.com/timetetng/tqq/napcat"
)

var (
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))

	chatStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	activeBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("67"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("110"))
)

type imageSentMsg struct {
	convIndex int
	filePath  string
	isTemp    bool
}

type Conversation struct {
	ID       int64
	Name     string
	Type     string
	Messages []napcat.Event
	Unread   int
}

type newMessageMsg napcat.Event

type previewResultMsg struct {
	content string
	isImg   bool
}

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
	showPreview   bool
	previewOutput string
	showSidebar   bool
	previewIsImg  bool
}

func NewApp(client *napcat.Client, selfID int64) AppModel {
	convs := loadHistory()

	selected := 0
	if len(convs) > 0 && len(convs[0].Messages) > 0 {
		selected = len(convs[0].Messages) - 1
	}

	return AppModel{
		client:      client,
		selfID:      selfID,
		convs:       convs,
		focus:       "sidebar",
		showSidebar: true,
		selectedMsg: selected,
	}
}

func (m AppModel) Init() tea.Cmd {
	return waitForMessage(m.client.Events)
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
		saveHistory(m.convs)
		return m, waitForMessage(m.client.Events)
	case previewResultMsg:
		if m.showPreview {
			m.previewOutput = msg.content
			m.previewIsImg = msg.isImg
		}
		return m, nil
	case imageSentMsg:
		var targetPath string
		if msg.isTemp {
			cacheDir := "/tmp/tqq_cache"
			os.MkdirAll(cacheDir, 0755)
			targetPath = filepath.Join(cacheDir, filepath.Base(msg.filePath))
			os.Rename(msg.filePath, targetPath)
		} else {
			targetPath = msg.filePath
		}

		if msg.convIndex < len(m.convs) {
			imgData := fmt.Sprintf(`{"file":"%s", "url":"local://%s"}`, filepath.Base(targetPath), targetPath)

			m.convs[msg.convIndex].Messages = append(m.convs[msg.convIndex].Messages, napcat.Event{
				MessageType: m.convs[msg.convIndex].Type,
				UserID:      m.selfID,
				RawMessage:  "[图片]",
				Message:     []napcat.Segment{{Type: "image", Data: []byte(imgData)}},
				Sender: napcat.Sender{
					UserID:   m.selfID,
					Nickname: "我",
				},
			})
			if m.current == msg.convIndex {
				m.selectedMsg = len(m.convs[m.current].Messages) - 1
			}
			saveHistory(m.convs)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "1" && !m.inputMode && !m.showPreview {
			m.showSidebar = !m.showSidebar
			if !m.showSidebar && m.focus == "sidebar" {
				m.focus = "chat"
			}
			return m, nil
		}

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
			m.selectedMsg = len(m.convs[m.current].Messages) - 1
			if m.selectedMsg < 0 {
				m.selectedMsg = 0
			}
		}
	case "k", "up":
		if m.current > 0 {
			m.current--
			m.selectedMsg = len(m.convs[m.current].Messages) - 1
			if m.selectedMsg < 0 {
				m.selectedMsg = 0
			}
		}
	case "enter", "l":
		m.focus = "chat"
		if m.current < len(m.convs) {
			m.convs[m.current].Unread = 0
			saveHistory(m.convs)
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
		return m, nil
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
	if m.current >= len(m.convs) {
		return nil
	}
	msgs := m.convs[m.current].Messages
	if m.selectedMsg >= len(msgs) {
		return nil
	}
	evt := msgs[m.selectedMsg]

	for _, seg := range evt.Message {
		if seg.Type != "image" {
			continue
		}
		var d napcat.ImageData
		json.Unmarshal(seg.Data, &d)
		url := napcat.GetImageURL(d.File, d.URL)
		if url == "" {
			return nil
		}

		m.showPreview = true
		m.previewOutput = "图片加载中...\n\n(按 'q' 返回)"
		m.previewIsImg = false

		return func() tea.Msg {
			var targetPath string

			if strings.HasPrefix(url, "local://") {
				targetPath = strings.TrimPrefix(url, "local://")
			} else {
				cacheDir := "/tmp/tqq_cache"
				os.MkdirAll(cacheDir, 0755)
				fileName := d.File
				if fileName == "" {
					fileName = "temp.jpg"
				}
				fileName = strings.ReplaceAll(fileName, "/", "_")
				fileName = strings.ReplaceAll(fileName, "\\", "_")
				targetPath = filepath.Join(cacheDir, fileName+".jpg")

				if _, err := os.Stat(targetPath); os.IsNotExist(err) {
					if err := exec.Command("curl", "-s", "-L", "-o", targetPath, url).Run(); err != nil {
						return previewResultMsg{"图片下载失败", false}
					}
				}
			}

			sidebarW := 0
			if m.showSidebar {
				sidebarW = m.width / 4
			}
			chatW := (m.width - sidebarW) / 2 - 2
			previewW := m.width - sidebarW - chatW - 4

			w, h := previewW-4, m.height-7
			if w <= 0 {
				w = 10
			}
			if h <= 0 {
				h = 10
			}
			sizeStr := fmt.Sprintf("%dx%d", w, h)

			out, err := exec.Command("chafa", "--format", "sixels", "--size", sizeStr, targetPath).Output()
			if err == nil && len(out) > 0 {
				return previewResultMsg{string(out), true}
			}
			out, _ = exec.Command("chafa", "--format", "symbols", "--size", sizeStr, targetPath).Output()
			return previewResultMsg{string(out), false}
		}
	}
	return nil
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
		return imageSentMsg{convIndex: convIndex, filePath: path, isTemp: true}
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
		return imageSentMsg{convIndex: convIndex, filePath: path, isTemp: false}
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
			isAtBottom := (m.selectedMsg == len(m.convs[i].Messages)-1)
			m.convs[i].Messages = append(m.convs[i].Messages, evt)
			if i != m.current || m.focus != "chat" {
				m.convs[i].Unread++
			} else if isAtBottom {
				m.selectedMsg = len(m.convs[i].Messages) - 1
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
	m.selectedMsg = len(m.convs[m.current].Messages) - 1
	saveHistory(m.convs)
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
		if !m.previewIsImg {
			content = "\n" + m.previewOutput
		}

		previewStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Width(previewW).Height(innerH).
			Align(lipgloss.Center)

		layout = append(layout, previewStyle.Render(content))
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top, layout...)
	input := m.renderInput(m.width)

	ui := lipgloss.JoinVertical(lipgloss.Left, top, input)

	if m.showPreview && m.previewIsImg {
		row := 3
		col := chatW + 4
		if m.showSidebar {
			col += sidebarW
		}
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
	var allLines []string
	selectedStart := 0
	selectedEnd := 0

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

		visualLines := strings.Split(strings.TrimRight(line, "\n"), "\n")

		if isSelected {
			selectedStart = len(allLines)
			selectedEnd = len(allLines) + len(visualLines) - 1
		}

		allLines = append(allLines, visualLines...)
	}

	viewH := h - 2
	if viewH <= 0 {
		return ""
	}

	viewTop := 0
	if len(allLines) > viewH {
		viewTop = selectedEnd - viewH + 1
		if viewTop > selectedStart {
			viewTop = selectedStart
		}
		if viewTop < 0 {
			viewTop = 0
		} else if viewTop+viewH > len(allLines) {
			viewTop = len(allLines) - viewH
		}
	}

	var visibleLines []string
	if len(allLines) > viewH {
		visibleLines = allLines[viewTop : viewTop+viewH]
	} else {
		visibleLines = allLines
	}

	content := strings.Join(visibleLines, "\n")

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
			Foreground(lipgloss.Color("36")).
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
			Background(lipgloss.Color("104")).
			Foreground(lipgloss.Color("0")).
			Bold(true).
			Render(" NORMAL ")
		prompt = "  "
		cursor = "▌"
	}

	inputArea := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("36")).
		Width(w - 12).
		Render(prompt + m.input + cursor)

	return lipgloss.JoinHorizontal(lipgloss.Center, modeTag, " ", inputArea)
}

func loadHistory() []Conversation {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, ".config", "tqq", "history.json")
	data, err := os.ReadFile(path)

	var convs []Conversation
	if err == nil {
		json.Unmarshal(data, &convs)
	}
	return convs
}

func saveHistory(convs []Conversation) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".config", "tqq", "history.json")
	os.MkdirAll(filepath.Dir(path), 0755)

	var cache []Conversation
	for _, c := range convs {
		cc := c
		if len(cc.Messages) > 50 {
			cc.Messages = cc.Messages[len(cc.Messages)-50:]
		}
		cache = append(cache, cc)
	}
	data, _ := json.Marshal(cache)
	os.WriteFile(path, data, 0644)
}
