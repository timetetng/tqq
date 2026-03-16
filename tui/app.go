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
	client      *napcat.Client
	selfID      int64
	convs       []Conversation
	current     int
	input       string
	focus       string
	inputMode   bool
	selectedMsg int
	width       int
	height      int
}

func NewApp(client *napcat.Client, selfID int64) AppModel {
	return AppModel{
		client: client,
		selfID: selfID,
		focus:  "sidebar",
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
		return m, waitForMessage(m.client.Events)
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
}   // ← 只有这一个 }，删掉下面多余的那个
func (m AppModel) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m AppModel) previewSelectedImage() tea.Cmd {
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
		return func() tea.Msg {
			go func() {
				tmp, err := os.CreateTemp("", "tqq-preview-*.jpg")
				if err != nil {
					return
				}
				tmp.Close()
				defer os.Remove(tmp.Name())
				if err := exec.Command("curl", "-s", "-L", "-o", tmp.Name(), url).Run(); err != nil {
					return
				}
				script := fmt.Sprintf(
					"chafa --fit-width --size 60x30 %s; echo ''; echo '按 Enter 关闭...'; read",
					tmp.Name(),
				)
				exec.Command("foot", "-T", "图片预览", "--", "sh", "-c", script).Run()
			}()
			return nil
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
	sidebarW := m.width / 4
	chatW := m.width - sidebarW - 4
	innerH := m.height - 5

	sidebar := m.renderSidebar(sidebarW, innerH)
	chat := m.renderChat(chatW, innerH)
	input := m.renderInput(m.width)

	top := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, chat)
	return lipgloss.JoinVertical(lipgloss.Left, top, input)
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

