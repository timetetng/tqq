package napcat

import "encoding/json"

type Event struct {
	PostType    string    `json:"post_type"`
	MessageType string    `json:"message_type"`
	Time        int64     `json:"time"`
	MessageID   int64     `json:"message_id"`
	UserID      int64     `json:"user_id"`
	GroupID     int64     `json:"group_id,omitempty"`
	GroupName   string    `json:"group_name,omitempty"`
	RawMessage  string    `json:"raw_message"`
	Message     []Segment `json:"message"`
	Sender      Sender    `json:"sender"`
}

type ImageData struct {
    File string `json:"file"`
    URL  string `json:"url"`
}


type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
}

type Segment struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type TextData struct {
	Text string `json:"text"`
}


type APIRequest struct {
	Action string      `json:"action"`
	Params interface{} `json:"params"`
	Echo   string      `json:"echo"`
}

type SendMsgParams struct {
	MessageType string    `json:"message_type"`
	UserID      int64     `json:"user_id,omitempty"`
	GroupID     int64     `json:"group_id,omitempty"`
	Message     []Segment `json:"message"`
}

