package napcat

import (
	"encoding/base64"
	"fmt"
	"os"
)

// 发送本地图片文件
func (c *Client) SendImage(msgType string, userID, groupID int64, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	b64 := base64.StdEncoding.EncodeToString(data)

	seg := Segment{Type: "image"}
	seg.Data = []byte(fmt.Sprintf(`{"file":"base64://%s"}`, b64))

	params := SendMsgParams{
		MessageType: msgType,
		Message:     []Segment{seg},
	}
	if msgType == "private" {
		params.UserID = userID
	} else {
		params.GroupID = groupID
	}
	return c.CallAPI("send_msg", params)
}

