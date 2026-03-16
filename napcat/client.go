package napcat

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn   *websocket.Conn
	Events chan Event
}

func NewClient(wsURL, token string) (*Client, error) {
	header := http.Header{}
	if token != "" {
		header.Add("Authorization", "Bearer "+token)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, Events: make(chan Event, 100)}
	go c.readLoop()
	return c, nil
}

func (c *Client) readLoop() {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			close(c.Events)
			return
		}
		var evt Event
		if json.Unmarshal(msg, &evt) == nil && (evt.PostType == "message" || evt.PostType == "message_sent") {
			c.Events <- evt
		}
	}
}

func (c *Client) CallAPI(action string, params interface{}) error {
	req := APIRequest{Action: action, Params: params, Echo: action}
	return c.conn.WriteJSON(req)
}

