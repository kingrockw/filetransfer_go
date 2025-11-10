package main

import (
	"crypto/rand"
	"encoding/hex"
)

// FileMetadata 文件元数据
type FileMetadata struct {
	FileName string `json:"fileName"`
	FileSize int64  `json:"fileSize"`
}

// Message 信令消息类型（用于WebRTC信令）
type Message struct {
	Type       string `json:"type"`        // "create_room", "join_room", "offer", "answer", "error"
	RoomID     string `json:"room_id,omitempty"`
	FileID     string `json:"file_id,omitempty"`
	SDP        string `json:"sdp,omitempty"`
	Error      string `json:"error,omitempty"`
	ClientType string `json:"client_type,omitempty"`
}

// generateFileID 生成随机文件ID
func generateFileID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

