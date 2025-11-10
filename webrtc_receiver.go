package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pion/webrtc/v3"
)

// WebRTCReceiver WebRTC文件接收端
type WebRTCReceiver struct {
	fileID       string
	sdpOffer     string
	savePath     string
	stunServer   string
	turnServer   string
	signalingURL string
	roomID       string
	pc           *webrtc.PeerConnection
	dc           *webrtc.DataChannel
	file         *os.File
	metadata     *FileMetadata
	state        int // 0: 等待元数据长度, 1: 等待元数据, 2: 接收文件数据
	metadataLen  uint32
	metadataBuf  []byte
	totalReceived int64
	startTime    time.Time
	debug        bool
}

// NewWebRTCReceiver 创建WebRTC接收端
func NewWebRTCReceiver(fileID, sdpOffer, savePath, stunServer, turnServer, signalingURL, roomID string, debug bool) *WebRTCReceiver {
	return &WebRTCReceiver{
		fileID:       fileID,
		sdpOffer:     sdpOffer,
		savePath:     savePath,
		stunServer:   stunServer,
		turnServer:   turnServer,
		signalingURL: signalingURL,
		roomID:       roomID,
		debug:        debug,
	}
}

// Start 开始接收文件
func (r *WebRTCReceiver) Start() error {
	fmt.Println("=== WebRTC P2P 文件传输 - 接收端 ===")
	fmt.Printf("文件编号: %s\n", r.fileID)

	// 配置ICE服务器
	iceServers := getDefaultICEServers(r.stunServer, r.turnServer, r.debug)

	// 创建PeerConnection配置
	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	// 创建PeerConnection
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("创建PeerConnection失败: %w", err)
	}
	r.pc = pc
	defer pc.Close()

	// 设置DataChannel接收事件
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		r.dc = dc
		r.state = 0
		r.startTime = time.Now()
		
		dc.OnOpen(func() {
			fmt.Println("DataChannel已打开，准备接收文件...")
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if err := r.handleMessage(msg.Data); err != nil {
				fmt.Printf("处理消息失败: %v\n", err)
			}
		})
	})

	// 设置ICE连接状态变化
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if r.debug {
			fmt.Printf("ICE连接状态: %s\n", state.String())
		}
		switch state {
		case webrtc.ICEConnectionStateConnected:
			if r.debug {
				fmt.Println("P2P连接已建立!")
			}
		case webrtc.ICEConnectionStateFailed, webrtc.ICEConnectionStateDisconnected, webrtc.ICEConnectionStateClosed:
			if r.debug {
				fmt.Printf("ICE连接失败: %s\n", state.String())
			}
		}
	})

	// 监听ICE候选者
	iceGatheringComplete := make(chan bool, 1)
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			// ICE候选者收集完成
			if r.debug {
				fmt.Println("ICE候选者收集完成")
			}
			select {
			case iceGatheringComplete <- true:
			default:
			}
		} else {
			if r.debug {
				fmt.Printf("ICE候选者: %s\n", candidate.String())
			}
		}
	})

	// 使用默认信令服务器（如果未指定）
	signalingURL := r.signalingURL
	if signalingURL == "" {
		signalingURL = getDefaultSignalingURL()
		if signalingURL != "" {
			if r.debug {
				fmt.Printf("使用默认信令服务器: %s\n", signalingURL)
			}
		}
	}

	// 处理Offer和Answer交换
	if signalingURL != "" {
		// 使用信令服务器
		if r.debug {
			fmt.Println("正在连接信令服务器...")
		}
		signalingClient, err := NewSignalingClient(signalingURL)
		if err != nil {
			return fmt.Errorf("连接信令服务器失败: %w", err)
		}
		defer signalingClient.Close()

		// 加入房间
		roomID := r.roomID
		if roomID == "" {
			roomID = r.fileID // 使用文件ID作为房间ID
		}

		fmt.Printf("加入房间: %s\n", roomID)
		signalingClient.Send(&Message{
			Type: "join_room",
			RoomID: roomID,
		})

		// 等待加入确认
		msg, err := signalingClient.Receive(5 * time.Second)
		if err != nil {
			return fmt.Errorf("等待加入房间失败: %w", err)
		}

		if msg.Type == "error" {
			return fmt.Errorf("加入房间失败: %s", msg.Error)
		}

		if msg.Type != "room_joined" {
			return fmt.Errorf("意外的消息类型: %s", msg.Type)
		}

		fmt.Println("已加入房间，等待Offer...")

		// 等待Offer
		var offerSDP string
		for {
			msg, err := signalingClient.Receive(5 * time.Minute)
			if err != nil {
				return fmt.Errorf("接收Offer失败: %w", err)
			}

			if msg.Type == "offer" {
				offerSDP = msg.SDP
				if msg.FileID != "" {
					r.fileID = msg.FileID
					fmt.Printf("文件编号: %s\n", r.fileID)
				}
				break
			} else if msg.Type == "error" {
				return fmt.Errorf("信令服务器错误: %s", msg.Error)
			}
		}

		// 解码Offer
		offerJSON, err := base64.StdEncoding.DecodeString(offerSDP)
		if err != nil {
			return fmt.Errorf("解码Offer失败: %w", err)
		}

		var offer webrtc.SessionDescription
		if err = json.Unmarshal(offerJSON, &offer); err != nil {
			return fmt.Errorf("解析Offer失败: %w", err)
		}

		// 打印SDP Offer信息（用于调试）
		if r.debug {
			fmt.Println("\n" + strings.Repeat("=", 70))
			fmt.Println("SDP Offer 信息:")
			fmt.Println(strings.Repeat("=", 70))
			fmt.Printf("类型: %s\n", offer.Type)
			fmt.Printf("SDP内容:\n%s\n", offer.SDP)
			fmt.Println(strings.Repeat("=", 70))
		}

		// 设置RemoteDescription
		if err = pc.SetRemoteDescription(offer); err != nil {
			return fmt.Errorf("设置RemoteDescription失败: %w", err)
		}

		// 创建Answer
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			return fmt.Errorf("创建Answer失败: %w", err)
		}

		// 设置LocalDescription（这会触发ICE候选者收集）
		if err = pc.SetLocalDescription(answer); err != nil {
			return fmt.Errorf("设置LocalDescription失败: %w", err)
		}

		// 等待ICE候选者收集完成
		if r.debug {
			fmt.Println("等待ICE候选者收集...")
		}
		select {
		case <-iceGatheringComplete:
			// 重新获取更新后的SDP（包含ICE候选者）
			answer = *pc.LocalDescription()
			if r.debug {
				fmt.Println("ICE候选者已收集完成")
			}
		case <-time.After(10 * time.Second):
			if r.debug {
				fmt.Println("警告: ICE候选者收集超时，继续使用当前SDP")
			}
		}

		// 打印SDP Answer信息（用于调试）
		if r.debug {
			fmt.Println("\n" + strings.Repeat("=", 70))
			fmt.Println("SDP Answer 信息:")
			fmt.Println(strings.Repeat("=", 70))
			fmt.Printf("类型: %s\n", answer.Type)
			fmt.Printf("SDP内容:\n%s\n", answer.SDP)
			fmt.Println(strings.Repeat("=", 70))
		}

		// 将Answer编码为base64
		answerJSON, err := json.Marshal(answer)
		if err != nil {
			return fmt.Errorf("序列化Answer失败: %w", err)
		}
		answerB64 := base64.StdEncoding.EncodeToString(answerJSON)

		// 发送Answer
		if r.debug {
			fmt.Println("Answer已创建，发送给发送端...")
		}
		signalingClient.Send(&Message{
			Type: "answer",
			RoomID: roomID,
			SDP: answerB64,
		})

		if r.debug {
			fmt.Println("Answer已发送，等待连接建立...")
		}
	} else {
		// 无信令服务器，使用手动输入方式
		if r.sdpOffer == "" {
			return fmt.Errorf("未提供SDP Offer（需要--signaling参数或手动提供SDP）")
		}

		// 解码Offer
		offerJSON, err := base64.StdEncoding.DecodeString(r.sdpOffer)
		if err != nil {
			return fmt.Errorf("解码Offer失败: %w", err)
		}

		var offer webrtc.SessionDescription
		if err = json.Unmarshal(offerJSON, &offer); err != nil {
			return fmt.Errorf("解析Offer失败: %w", err)
		}

		// 打印SDP Offer信息（用于调试）
		if r.debug {
			fmt.Println("\n" + strings.Repeat("=", 70))
			fmt.Println("SDP Offer 信息:")
			fmt.Println(strings.Repeat("=", 70))
			fmt.Printf("类型: %s\n", offer.Type)
			fmt.Printf("SDP内容:\n%s\n", offer.SDP)
			fmt.Println(strings.Repeat("=", 70))
		}

		// 设置RemoteDescription
		if err = pc.SetRemoteDescription(offer); err != nil {
			return fmt.Errorf("设置RemoteDescription失败: %w", err)
		}

		// 创建Answer
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			return fmt.Errorf("创建Answer失败: %w", err)
		}

		// 设置LocalDescription（这会触发ICE候选者收集）
		if err = pc.SetLocalDescription(answer); err != nil {
			return fmt.Errorf("设置LocalDescription失败: %w", err)
		}

		// 等待ICE候选者收集完成
		if r.debug {
			fmt.Println("等待ICE候选者收集...")
		}
		select {
		case <-iceGatheringComplete:
			// 重新获取更新后的SDP（包含ICE候选者）
			answer = *pc.LocalDescription()
			if r.debug {
				fmt.Println("ICE候选者已收集完成")
			}
		case <-time.After(10 * time.Second):
			if r.debug {
				fmt.Println("警告: ICE候选者收集超时，继续使用当前SDP")
			}
		}

		// 打印SDP Answer信息（用于调试）
		if r.debug {
			fmt.Println("\n" + strings.Repeat("=", 70))
			fmt.Println("SDP Answer 信息:")
			fmt.Println(strings.Repeat("=", 70))
			fmt.Printf("类型: %s\n", answer.Type)
			fmt.Printf("SDP内容:\n%s\n", answer.SDP)
			fmt.Println(strings.Repeat("=", 70))
		}
		fmt.Println(strings.Repeat("=", 70))
		fmt.Printf("类型: %s\n", answer.Type)
		fmt.Printf("SDP内容:\n%s\n", answer.SDP)
		fmt.Println(strings.Repeat("=", 70))

		// 将Answer编码为base64
		answerJSON, err := json.Marshal(answer)
		if err != nil {
			return fmt.Errorf("序列化Answer失败: %w", err)
		}
		answerB64 := base64.StdEncoding.EncodeToString(answerJSON)

		// 显示Answer
		if r.debug {
			fmt.Println("\n" + strings.Repeat("=", 70))
			fmt.Println("请将以下Answer发送给发送端:")
			fmt.Println(strings.Repeat("=", 70))
			fmt.Printf("SDP Answer (base64):\n%s\n", answerB64)
			fmt.Println(strings.Repeat("=", 70))
			fmt.Println("\n等待文件传输...")
		}
	}

	// 等待文件接收完成
	select {
	case <-time.After(30 * time.Minute):
		return fmt.Errorf("文件接收超时")
	}
}

// handleMessage 处理接收到的消息
func (r *WebRTCReceiver) handleMessage(data []byte) error {
	switch r.state {
	case 0: // 等待元数据长度
		if len(data) >= 4 {
			r.metadataLen = binary.BigEndian.Uint32(data[:4])
			r.metadataBuf = make([]byte, 0, r.metadataLen)
			r.state = 1
			if len(data) > 4 {
				// 如果还有数据，继续处理
				return r.handleMessage(data[4:])
			}
		}
	case 1: // 等待元数据
		r.metadataBuf = append(r.metadataBuf, data...)
		if len(r.metadataBuf) >= int(r.metadataLen) {
			// 解析元数据
			var metadata FileMetadata
			if err := json.Unmarshal(r.metadataBuf[:r.metadataLen], &metadata); err != nil {
				return fmt.Errorf("解析元数据失败: %w", err)
			}
			r.metadata = &metadata

			fmt.Printf("文件: %s\n", metadata.FileName)
			fmt.Printf("大小: %d 字节 (%.2f MB)\n", metadata.FileSize, float64(metadata.FileSize)/1024/1024)

			// 确定保存路径
			savePath := r.savePath
			if savePath == "" || savePath == "." {
				savePath = metadata.FileName
			} else {
				if info, err := os.Stat(savePath); err == nil && info.IsDir() {
					savePath = filepath.Join(savePath, metadata.FileName)
				} else if err != nil && os.IsNotExist(err) {
					// savePath可能是目录但不存在，尝试创建
					if err := os.MkdirAll(savePath, 0755); err == nil {
						savePath = filepath.Join(savePath, metadata.FileName)
					}
				}
			}

			// 确保保存目录存在
			dir := filepath.Dir(savePath)
			if dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("创建保存目录失败: %w", err)
				}
			}

			// 保存完整路径用于后续显示
			r.savePath = savePath

			// 创建文件
			file, err := os.Create(savePath)
			if err != nil {
				return fmt.Errorf("创建文件失败: %w", err)
			}
			r.file = file

			fmt.Printf("保存到: %s\n", savePath)
			fmt.Println("开始接收...")
			fmt.Println()

			r.state = 2

			// 如果还有剩余数据，继续处理
			if len(r.metadataBuf) > int(r.metadataLen) {
				return r.handleMessage(r.metadataBuf[r.metadataLen:])
			}
		}
	case 2: // 接收文件数据
		if r.file == nil {
			return fmt.Errorf("文件未创建")
		}

		written, err := r.file.Write(data)
		if err != nil {
			return fmt.Errorf("写入文件失败: %w", err)
		}

		r.totalReceived += int64(written)

		// 显示进度
		if r.metadata != nil && r.metadata.FileSize > 0 {
			progress := float64(r.totalReceived) / float64(r.metadata.FileSize) * 100
			elapsed := time.Since(r.startTime).Seconds()
			if elapsed > 0 {
				speed := float64(r.totalReceived) / elapsed / 1024 / 1024 // MB/s
				fmt.Printf("\r进度: %.2f%% (%.2f MB/s)", progress, speed)
			}

			// 检查是否接收完成
			if r.totalReceived >= r.metadata.FileSize {
				r.file.Close()
				elapsed := time.Since(r.startTime).Seconds()
				
				// 获取文件的绝对路径
				absPath, _ := filepath.Abs(r.savePath)
				
				fmt.Println("\n" + strings.Repeat("=", 70))
				fmt.Println("✓ 接收完成!")
				fmt.Println(strings.Repeat("=", 70))
				fmt.Printf("文件保存路径: %s\n", absPath)
				fmt.Printf("总大小: %d 字节 (%.2f MB)\n", r.totalReceived, float64(r.totalReceived)/1024/1024)
				fmt.Printf("耗时: %.2f 秒\n", elapsed)
				if elapsed > 0 {
					fmt.Printf("平均速度: %.2f MB/s\n", float64(r.totalReceived)/elapsed/1024/1024)
				}
				fmt.Println(strings.Repeat("=", 70))
				
				// 发送确认消息给发送端
				if r.dc != nil && r.dc.ReadyState() == webrtc.DataChannelStateOpen {
					ack := map[string]string{"type": "file_received"}
					ackJSON, _ := json.Marshal(ack)
					if err := r.dc.Send(ackJSON); err != nil {
						fmt.Printf("发送确认消息失败: %v\n", err)
					} else {
						fmt.Println("已发送接收完成确认给发送端，可以关闭窗口了（按Ctrl+C退出）")
					}
				}
				
				// 等待一小段时间确保确认消息发送完成
				time.Sleep(500 * time.Millisecond)
				return nil // 接收完成，不再处理后续消息
			}
		} else {
			elapsed := time.Since(r.startTime).Seconds()
			if elapsed > 0 {
				speed := float64(r.totalReceived) / elapsed / 1024 / 1024 // MB/s
				fmt.Printf("\r已接收: %.2f MB (%.2f MB/s)", float64(r.totalReceived)/1024/1024, speed)
			}
		}
	}

	return nil
}

