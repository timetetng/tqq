package napcat

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"strings"
)


var httpAPIBase string
var httpToken string

func SetHTTPAPI(base, token string) {
	httpAPIBase = base
	httpToken = token
}

func RenderMessage(segs []Segment) string {
	var parts []string
	for _, seg := range segs {
		parts = append(parts, renderSegment(seg))
	}
	return strings.Join(parts, "")
}

func renderSegment(seg Segment) string {
	switch seg.Type {
	case "text":
		var d TextData
		json.Unmarshal(seg.Data, &d)
		return d.Text

case "image":
    var d ImageData
    json.Unmarshal(seg.Data, &d)
    return "📷 [图片 按Enter预览]"


	case "face":
		var d struct {
			ID string `json:"id"`
		}
		json.Unmarshal(seg.Data, &d)
		return fmt.Sprintf("[表情:%s]", d.ID)

	case "at":
		var d struct {
			QQ string `json:"qq"`
		}
		json.Unmarshal(seg.Data, &d)
		return fmt.Sprintf("@%s", d.QQ)

	case "reply":
		var d struct {
			ID string `json:"id"`
		}
		json.Unmarshal(seg.Data, &d)
		return fmt.Sprintf("[回复:%s] ", d.ID)

	default:
		return fmt.Sprintf("[%s]", seg.Type)
	}
}

func renderImage(d ImageData) string {
	url := GetImageURL(d.File, d.URL)
	if url == "" {
		return "[图片]"
	}

	tmp, err := os.CreateTemp("", "tqq-*.jpg")
	if err != nil {
		return "[图片]"
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := exec.Command("curl", "-s", "-L", "-o", tmp.Name(), url).Run(); err != nil {
		return "[图片(下载失败)]"
	}

	// 按优先级依次尝试，第一个成功就返回
	renderers := []struct {
		name string
		args []string
	}{
		{"chafa", []string{"--format", "kitty", "--size", "40x20", tmp.Name()}},
		{"chafa", []string{"--format", "sixels", "--size", "40x20", tmp.Name()}},
		{"img2sixel", []string{"-w", "300", tmp.Name()}},
		{"chafa", []string{"--format", "symbols", "--size", "40x20", tmp.Name()}}, // 终极兜底，所有终端都能用
	}

	for _, r := range renderers {
		out, err := exec.Command(r.name, r.args...).Output()
		if err == nil && len(out) > 0 {
			return "\n" + string(out) + "\n"
		}
	}

	return "[图片(渲染失败，请安装 chafa)]"
}


// 通过 NapCat API 拿到最新的带 rkey 的 URL
func GetImageURL(file, fallbackURL string) string {
	if httpAPIBase != "" && file != "" {
		apiURL := fmt.Sprintf("%s/get_image?file=%s", httpAPIBase, file)
		if httpToken != "" {
			apiURL += "&access_token=" + httpToken
		}
		out, err := exec.Command("curl", "-s", apiURL).Output()
		if err == nil {
			var resp struct {
				Data struct {
					URL string `json:"url"`
				} `json:"data"`
				RetCode int `json:"retcode"`
			}
			if json.Unmarshal(out, &resp) == nil && resp.Data.URL != "" {
				return html.UnescapeString(resp.Data.URL)
			}
		}
	}
	// 降级用消息里自带的 URL
	return html.UnescapeString(fallbackURL)
}

