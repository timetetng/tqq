package main

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/timetetng/tqq/config"
	"github.com/timetetng/tqq/napcat"
	"github.com/timetetng/tqq/tui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("配置加载失败:", err)
	}
	napcat.SetHTTPAPI(cfg.NapCat.HTTPURL, cfg.NapCat.Token)

	client, err := napcat.NewClient(cfg.NapCat.WSURL, cfg.NapCat.Token)
	if err != nil {
		log.Fatal("连接失败:", err)
	}
	fmt.Println("连接成功")

	// 从配置读取自己的 QQ 号，用于区分消息左右对齐
	app := tui.NewApp(client, cfg.NapCat.SelfID)

	p := tea.NewProgram(app, tea.WithAltScreen())
	
	// 捕获程序退出时的最终状态
	m, err := p.Run()
	if err != nil {
		log.Fatal(err)
	}

	// 程序退出时统一执行一次文件保存，彻底解决高 I/O 问题
	if finalApp, ok := m.(tui.AppModel); ok {
		finalApp.SaveHistory()
	}
}
