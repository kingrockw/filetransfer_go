package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// SignalingClient 信令客户端
type SignalingClient struct {
	conn   *websocket.Conn
	send   chan *Message
	recv   chan *Message
	errors chan error
}

// NewSignalingClient 创建信令客户端
func NewSignalingClient(serverURL string) (*SignalingClient, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("解析服务器URL失败: %w", err)
	}

	// 确保使用WebSocket协议
	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	} else if u.Scheme == "" {
		u.Scheme = "ws"
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("连接信令服务器失败: %w", err)
	}

	client := &SignalingClient{
		conn:   conn,
		send:   make(chan *Message, 256),
		recv:   make(chan *Message, 256),
		errors: make(chan error, 1),
	}

	go client.readPump()
	go client.writePump()

	return client, nil
}

// readPump 读取消息
func (c *SignalingClient) readPump() {
	defer close(c.recv)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			c.errors <- err
			return
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("解析消息失败: %v", err)
			continue
		}

		c.recv <- &msg
	}
}

// writePump 发送消息
func (c *SignalingClient) writePump() {
	defer c.conn.Close()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("序列化消息失败: %v", err)
				continue
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("发送消息失败: %v", err)
				return
			}
		}
	}
}

// Send 发送消息
func (c *SignalingClient) Send(msg *Message) {
	select {
	case c.send <- msg:
	default:
		log.Printf("发送队列已满，丢弃消息")
	}
}

// Receive 接收消息（带超时）
func (c *SignalingClient) Receive(timeout time.Duration) (*Message, error) {
	select {
	case msg := <-c.recv:
		return msg, nil
	case err := <-c.errors:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("接收消息超时")
	}
}

// Close 关闭连接
func (c *SignalingClient) Close() {
	close(c.send)
	c.conn.Close()
}

