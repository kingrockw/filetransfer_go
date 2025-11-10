package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 信令服务器
type SignalingServer struct {
	rooms    map[string]*Room
	roomsMu  sync.RWMutex
	upgrader websocket.Upgrader
}

// Room 房间
type Room struct {
	ID        string
	clients   map[*Client]bool
	clientsMu sync.RWMutex
	createdAt time.Time
}

// Client 客户端
type Client struct {
	conn     *websocket.Conn
	room     *Room
	send     chan []byte
	server   *SignalingServer
	clientType string // "sender" or "receiver"
}

// Message 消息类型
type Message struct {
	Type      string `json:"type"`      // "create_room", "join_room", "offer", "answer", "error"
	RoomID    string `json:"room_id,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	SDP       string `json:"sdp,omitempty"`
	Error     string `json:"error,omitempty"`
	ClientType string `json:"client_type,omitempty"`
}

// NewSignalingServer 创建信令服务器
func NewSignalingServer() *SignalingServer {
	return &SignalingServer{
		rooms: make(map[string]*Room),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源（简单实现，不检查来源）
			},
		},
	}
}

// NewRoom 创建新房间
func (s *SignalingServer) NewRoom(roomID string) *Room {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()

	room := &Room{
		ID:        roomID,
		clients:   make(map[*Client]bool),
		createdAt: time.Now(),
	}
	s.rooms[roomID] = room
	return room
}

// GetRoom 获取房间
func (s *SignalingServer) GetRoom(roomID string) *Room {
	s.roomsMu.RLock()
	defer s.roomsMu.RUnlock()
	return s.rooms[roomID]
}

// RemoveRoom 移除房间（当房间为空时）
func (s *SignalingServer) RemoveRoom(roomID string) {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()
	delete(s.rooms, roomID)
}

// handleWebSocket 处理WebSocket连接
func (s *SignalingServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
		server: s,
	}

	go client.writePump()
	go client.readPump()
}

// readPump 读取客户端消息
func (c *Client) readPump() {
	defer func() {
		c.conn.Close()
		if c.room != nil {
			c.leaveRoom()
		}
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket错误: %v", err)
			}
			break
		}

		c.handleMessage(message)
	}
}

// writePump 向客户端发送消息
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 发送队列中的其他消息
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage 处理客户端消息
func (c *Client) handleMessage(data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.sendError("无效的消息格式")
		return
	}

	switch msg.Type {
	case "create_room":
		c.handleCreateRoom(&msg)
	case "join_room":
		c.handleJoinRoom(&msg)
	case "offer":
		c.handleOffer(&msg)
	case "answer":
		c.handleAnswer(&msg)
	default:
		c.sendError(fmt.Sprintf("未知的消息类型: %s", msg.Type))
	}
}

// handleCreateRoom 处理创建房间
func (c *Client) handleCreateRoom(msg *Message) {
	if msg.RoomID == "" {
		c.sendError("房间ID不能为空")
		return
	}

	// 检查房间是否已存在
	room := c.server.GetRoom(msg.RoomID)
	if room != nil {
		c.sendError("房间已存在")
		return
	}

	// 创建房间
	room = c.server.NewRoom(msg.RoomID)
	c.room = room
	c.clientType = "sender"
	room.clientsMu.Lock()
	room.clients[c] = true
	room.clientsMu.Unlock()

	log.Printf("房间 %s 已创建，客户端类型: sender", msg.RoomID)

	// 发送确认
	response := Message{
		Type: "room_created",
		RoomID: msg.RoomID,
	}
	c.sendMessage(&response)
}

// handleJoinRoom 处理加入房间
func (c *Client) handleJoinRoom(msg *Message) {
	if msg.RoomID == "" {
		c.sendError("房间ID不能为空")
		return
	}

	room := c.server.GetRoom(msg.RoomID)
	if room == nil {
		c.sendError("房间不存在")
		return
	}

	c.room = room
	c.clientType = "receiver"
	room.clientsMu.Lock()
	room.clients[c] = true
	room.clientsMu.Unlock()

	log.Printf("客户端加入房间 %s，客户端类型: receiver", msg.RoomID)

	// 发送确认
	response := Message{
		Type: "room_joined",
		RoomID: msg.RoomID,
	}
	c.sendMessage(&response)

	// 通知房间内其他客户端有新成员加入
	c.broadcastToRoom(Message{
		Type: "peer_joined",
		RoomID: msg.RoomID,
	}, c)
}

// handleOffer 处理Offer
func (c *Client) handleOffer(msg *Message) {
	if c.room == nil {
		c.sendError("未加入房间")
		return
	}

	if c.clientType != "sender" {
		c.sendError("只有发送端可以发送Offer")
		return
	}

	// 广播Offer给房间内其他客户端（接收端）
	c.broadcastToRoom(Message{
		Type: "offer",
		RoomID: msg.RoomID,
		FileID: msg.FileID,
		SDP: msg.SDP,
	}, c)
}

// handleAnswer 处理Answer
func (c *Client) handleAnswer(msg *Message) {
	if c.room == nil {
		c.sendError("未加入房间")
		return
	}

	if c.clientType != "receiver" {
		c.sendError("只有接收端可以发送Answer")
		return
	}

	// 广播Answer给房间内其他客户端（发送端）
	c.broadcastToRoom(Message{
		Type: "answer",
		RoomID: msg.RoomID,
		SDP: msg.SDP,
	}, c)
}

// broadcastToRoom 向房间内其他客户端广播消息
func (c *Client) broadcastToRoom(msg Message, exclude *Client) {
	if c.room == nil {
		return
	}

	c.room.clientsMu.RLock()
	defer c.room.clientsMu.RUnlock()

	for client := range c.room.clients {
		if client != exclude {
			client.sendMessage(&msg)
		}
	}
}

// leaveRoom 离开房间
func (c *Client) leaveRoom() {
	if c.room == nil {
		return
	}

	c.room.clientsMu.Lock()
	delete(c.room.clients, c)
	clientCount := len(c.room.clients)
	c.room.clientsMu.Unlock()

	log.Printf("客户端离开房间 %s，剩余客户端: %d", c.room.ID, clientCount)

	// 如果房间为空，移除房间
	if clientCount == 0 {
		c.server.RemoveRoom(c.room.ID)
		log.Printf("房间 %s 已移除（无客户端）", c.room.ID)
	} else {
		// 通知其他客户端有成员离开
		c.broadcastToRoom(Message{
			Type: "peer_left",
			RoomID: c.room.ID,
		}, c)
	}

	c.room = nil
}

// sendMessage 发送消息
func (c *Client) sendMessage(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("序列化消息失败: %v", err)
		return
	}

	select {
	case c.send <- data:
	default:
		close(c.send)
	}
}

// sendError 发送错误消息
func (c *Client) sendError(errMsg string) {
	msg := Message{
		Type: "error",
		Error: errMsg,
	}
	c.sendMessage(&msg)
}

// Start 启动信令服务器
func (s *SignalingServer) Start(port int) error {
	http.HandleFunc("/ws", s.handleWebSocket)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("WebRTC信令服务器运行中\n"))
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("信令服务器启动在端口 %d", port)
	log.Printf("WebSocket端点: ws://localhost:%d/ws", port)
	return http.ListenAndServe(addr, nil)
}

