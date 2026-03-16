package main

import (
	"fmt"
	"log"

	"github.com/timetetng/tqq/config"
	"github.com/timetetng/tqq/napcat"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("配置加载失败:", err)
	}
	client, err := napcat.NewClient(cfg.NapCat.WSURL, cfg.NapCat.Token)
	if err != nil {
		log.Fatal("连接失败:", err)
	}
	fmt.Println("连接成功，等待消息...")

	for evt := range client.Events {
		name := evt.Sender.Card
		if name == "" {
			name = evt.Sender.Nickname
		}
		fmt.Printf("[%s] %s: %s\n", evt.MessageType, name, evt.RawMessage)
	}
}

