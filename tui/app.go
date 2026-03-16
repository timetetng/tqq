package tui

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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
	content   string
	isImg     bool
	imgWidth  int
	imgHeight int
}

type AppModel struct {
	client           *napcat.Client
	selfID           int64
	convs            []Conversation
	current          int
	input            string
	focus            string
	inputMode        bool
	selectedMsg      int
	width            int
	height           int
	showPreview      bool
	previewOutput    string
	showSidebar      bool
	previewIsImg     bool
	previewImgWidth  int
	previewImgHeight int
}

// 探测当前终端是否可能支持图片协议
func isTermImageSupported() bool {
    term := os.Getenv("TERM")
    termProg := os.Getenv("TERM_PROGRAM")
    return strings.Contains(term, "kitty") ||
        strings.Contains(term, "wezterm") ||
        strings.Contains(term, "sixel") ||
        strings.Contains(term, "mlterm") ||
        strings.Contains(term, "st") || // ✅ 加入对 st 的支持
        term == "foot" || term == "xterm-ghostty" ||
        termProg == "WezTerm" || termProg == "iTerm.app" || termProg == "MacTerm"
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
			m.previewImgWidth = msg.imgWidth
			m.previewImgHeight = msg.imgHeight
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
		case "q", "esc", "enter": // 退出预览时触发一次清屏，清除图片残影
			m.showPreview = false
			m.previewOutput = ""
			m.previewIsImg = false
			return m, tea.ClearScreen
		case "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.current < len(m.convs) {
				msgs := m.convs[m.current].Messages
				if m.selectedMsg < len(msgs)-1 {
					m.selectedMsg++
					return m, m.previewSelectedImage() // 触发新消息预览
				}
			}
			return m, nil
		case "k", "up":
			if m.selectedMsg > 0 {
				m.selectedMsg--
				return m, m.previewSelectedImage() // 触发新消息预览
			}
			return m, nil
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

	var imgSeg *napcat.Segment
	for i := range evt.Message {
		if evt.Message[i].Type == "image" {
			imgSeg = &evt.Message[i]
			break
		}
	}

	// 如果滚动到的消息没有图片，提示并清屏擦除老图片
	if imgSeg == nil {
		m.showPreview = true
		m.previewOutput = "当前消息无图片\n\n(jk切换，q退出)"
		m.previewIsImg = false
		return tea.ClearScreen
	}

	var d napcat.ImageData
	json.Unmarshal(imgSeg.Data, &d)
	url := napcat.GetImageURL(d.File, d.URL)
	if url == "" {
		m.showPreview = true
		m.previewOutput = "图片链接无效\n\n(jk切换，q退出)"
		m.previewIsImg = false
		return tea.ClearScreen
	}

	m.showPreview = true
	m.previewOutput = "图片加载中...\n\n(jk切换，q退出)"
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
					return previewResultMsg{"图片下载失败", false, 0, 0}
				}
			}
		}

		sidebarOuterW := 0
		if m.showSidebar {
			sidebarOuterW = m.width / 4
		}
		remainingOuter := m.width - sidebarOuterW
		chatOuterW := remainingOuter * 2 / 3
		previewOuterW := remainingOuter - chatOuterW
		
		previewInnerW := previewOuterW - 2

		w := previewInnerW - 6
		h := m.height - 12
		if w <= 10 { w = 10 }
		if h <= 5 { h = 5 }

		actualW := w
		actualH := h
		if file, err := os.Open(targetPath); err == nil {
			if imgConfig, _, err := image.DecodeConfig(file); err == nil {
				imgW := float64(imgConfig.Width)
				imgH := float64(imgConfig.Height)
				if imgW > 0 && imgH > 0 {
					spaceRatio := float64(w) / float64(h*2)
					imgRatio := imgW / imgH
					if imgRatio < spaceRatio {
						actualW = int(imgRatio * float64(h*2))
						actualH = h
					} else {
						actualW = w
						actualH = int(float64(w) / (imgRatio * 2))
					}
				}
			}
			file.Close()
		}
		if actualW < 1 { actualW = 1 }
		if actualH < 1 { actualH = 1 }

		sizeStr := fmt.Sprintf("%dx%d", w, h)            
		var out []byte
		var err error
		isImg := false

		if isTermImageSupported() {
			format := "sixels"
			term := os.Getenv("TERM")
			termProg := os.Getenv("TERM_PROGRAM")
			
			if strings.Contains(term, "kitty") || termProg == "WezTerm" {
				format = "kitty"
			}

			out, err = exec.Command("chafa", "--format", format, "--size", sizeStr, targetPath).Output()
			if err == nil && len(out) > 0 {
				isImg = true
			}
		}

		if !isImg {
			out, _ = exec.Command("chafa", "--format", "symbols", "--symbols", "half", "--size", sizeStr, targetPath).Output()
		}

		return previewResultMsg{string(out), isImg, actualW, actualH}
	}
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

	sidebarOuterW := 0
	if m.showSidebar {
		sidebarOuterW = m.width / 4
	}

	var chatOuterW, previewOuterW int
	if m.showPreview {
		remainingOuter := m.width - sidebarOuterW
		chatOuterW = remainingOuter * 2 / 3 
		previewOuterW = remainingOuter - chatOuterW
	} else {
		chatOuterW = m.width - sidebarOuterW
	}

	sidebarInnerW := sidebarOuterW - 2
	if sidebarInnerW < 1 { sidebarInnerW = 1 }
	chatInnerW := chatOuterW - 2
	if chatInnerW < 1 { chatInnerW = 1 }
	previewInnerW := previewOuterW - 2
	if previewInnerW < 1 { previewInnerW = 1 }

	innerH := m.height - 5
	var layout []string

	if m.showSidebar {
		layout = append(layout, m.renderSidebar(sidebarInnerW, innerH))
	}
	layout = append(layout, m.renderChat(chatInnerW, innerH))

	if m.showPreview {
		content := ""
		if !m.previewIsImg {
			content = "\n" + m.previewOutput
		}

		previewStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Width(previewInnerW).Height(innerH).
			Align(lipgloss.Center)

		layout = append(layout, previewStyle.Render(content))
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top, layout...)
	input := m.renderInput(m.width)

	ui := lipgloss.JoinVertical(lipgloss.Left, top, input)

if m.showPreview && m.previewIsImg {
		actualW := m.previewImgWidth
		actualH := m.previewImgHeight
		if actualW <= 0 { actualW = previewInnerW - 6 }
		if actualH <= 0 { actualH = innerH - 7 }

		// 水平・垂直のセンタリングオフセットを計算
		colOffset := (previewInnerW - actualW) / 2
		if colOffset < 0 { colOffset = 0 }
		rowOffset := (innerH - actualH) / 2
		if rowOffset < 0 { rowOffset = 0 }

		// 行: 内枠の起点(2) + 垂直オフセット
		row := 2 + rowOffset 
		// 列: サイドバー幅 + チャット枠幅 + 内枠起点(2) + 水平オフセット
		col := sidebarOuterW + chatOuterW + 2 + colOffset 

		safeImgOutput := strings.TrimSpace(m.previewOutput)
		safeImgOutput = strings.ReplaceAll(safeImgOutput, "\n", "")
		safeImgOutput = strings.ReplaceAll(safeImgOutput, "\r", "")

		// 【关键修复】构造清理序列：利用终端 ANSI 移动光标并填充空格，把预览框内部空间物理擦除一次
		clearSeq := ""
		startRow := 2
		startCol := sidebarOuterW + chatOuterW + 2
		for r := 0; r < innerH; r++ {
			clearSeq += fmt.Sprintf("\x1b[%d;%dH%s", startRow+r, startCol, strings.Repeat(" ", previewInnerW))
		}
		
		kittyClear := "\x1b_a=d\x1b\\" // Kitty 协议全清指令（非Kitty终端会忽略）

		// 先清屏 (kittyClear + clearSeq)，再将光标移到正确位置画图
		overlay := fmt.Sprintf("\x1b7%s%s\x1b[?7l\x1b[?80l\x1b[%d;%dH%s\x1b[?80h\x1b[?7h\x1b8", kittyClear, clearSeq, row, col, safeImgOutput)
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
	wrapStyle := lipgloss.NewStyle().Width(w)

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

		wrappedLine := wrapStyle.Render(line)
		visualLines := strings.Split(strings.TrimRight(wrappedLine, "\n"), "\n")

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
