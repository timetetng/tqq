package media

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// 从 Wayland 剪贴板读取图片，保存到临时文件，返回路径
func GetClipboardImage() (string, error) {
	// 检查剪贴板是否有图片
	types, err := exec.Command("wl-paste", "--list-types").Output()
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(types), "image/png") &&
		!strings.Contains(string(types), "image/jpeg") {
		return "", fmt.Errorf("剪贴板没有图片")
	}

	// 读取图片数据
	data, err := exec.Command("wl-paste", "--type", "image/png").Output()
	if err != nil {
		// 降级尝试 jpeg
		data, err = exec.Command("wl-paste", "--type", "image/jpeg").Output()
		if err != nil {
			return "", err
		}
	}

	tmp, err := os.CreateTemp("", "tqq-clip-*.png")
	if err != nil {
		return "", err
	}
	tmp.Write(data)
	tmp.Close()
	return tmp.Name(), nil
}

// 用 zenity 打开文件选择器，返回选中的文件路径
func PickImageFile() (string, error) {
	out, err := exec.Command("zenity",
		"--file-selection",
		"--title=选择图片",
		"--file-filter=图片 | *.png *.jpg *.jpeg *.gif *.webp",
	).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

