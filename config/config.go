package config

import (
    "os"
    "path/filepath"
    "github.com/BurntSushi/toml"
)

type Config struct {
    NapCat NapCatConfig `toml:"napcat"`
    UI     UIConfig     `toml:"ui"`
}

type NapCatConfig struct {
    WSURL   string `toml:"ws_url"`
    HTTPURL string `toml:"http_url"`   
		Token   string `toml:"token"`
    SelfID  int64  `toml:"self_id"`
}


type UIConfig struct {
    ImageWidth  int  `toml:"image_width"`   // Sixel 渲染宽度（px），默认200
    ShowAvatar  bool `toml:"show_avatar"`   // 是否显示头像
}



// 默认配置
var Default = Config{
    NapCat: NapCatConfig{
        WSURL: "ws://localhost:3001",
        Token: "",
    },
    UI: UIConfig{
        ImageWidth: 200,
        ShowAvatar: true,
    },
}

func Load() (Config, error) {
    cfg := Default
    path := filepath.Join(os.Getenv("HOME"), ".config", "tqq", "config.toml")
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return cfg, nil // 文件不存在就用默认值
    }
    _, err := toml.DecodeFile(path, &cfg)
    return cfg, err
}

