package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pion/webrtc/v3"
)

// WebRTCSender WebRTC文件发送端
type WebRTCSender struct {
	filePath      string
	stunServer    string
	turnServer    string
	signalingURL  string
	roomID        string
	fileID        string
	pc            *webrtc.PeerConnection
	dc            *webrtc.DataChannel
	debug         bool
}

// NewWebRTCSender 创建WebRTC发送端
func NewWebRTCSender(filePath, stunServer, turnServer, signalingURL, roomID string) *WebRTCSender {
	return &WebRTCSender{
		filePath:     filePath,
		stunServer:   stunServer,
		turnServer:   turnServer,
		signalingURL: signalingURL,
		roomID:       roomID,
	}
}

// Start 开始发送文件
func (s *WebRTCSender) Start() error {
	// 检查文件是否存在
	fileInfo, err := os.Stat(s.filePath)
	if err != nil {
		return fmt.Errorf("文件不存在: %w", err)
	}

	fileName := filepath.Base(s.filePath)
	fileSize := fileInfo.Size()

	// 生成随机文件ID（如果尚未设置）
	if s.fileID == "" {
		s.fileID = generateFileID()
	}

	fmt.Println("=== WebRTC P2P 文件传输 - 发送端 ===")

	// 配置ICE服务器
	iceServers := getDefaultICEServers(s.stunServer, s.turnServer, s.debug)

	// 创建PeerConnection配置
	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	// 创建PeerConnection
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("创建PeerConnection失败: %w", err)
	}
	s.pc = pc
	defer pc.Close()

	// 创建DataChannel
	ordered := true
	dc, err := pc.CreateDataChannel("fileTransfer", &webrtc.DataChannelInit{
		Ordered: &ordered, // 保证顺序
	})
	if err != nil {
		return fmt.Errorf("创建DataChannel失败: %w", err)
	}
	s.dc = dc

	// 设置DataChannel打开事件
	fileSentChan := make(chan bool, 1)
	fileReceivedAck := make(chan bool, 1) // 接收端确认接收完成
	
	// 监听接收端的消息（用于接收确认）
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var ack struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg.Data, &ack); err == nil {
			if ack.Type == "file_received" {
				fmt.Println("\n接收端已确认接收完成")
				select {
				case fileReceivedAck <- true:
				default:
				}
			}
		}
	})
	
	dc.OnOpen(func() {
		fmt.Println("DataChannel已打开，开始传输文件...")
		go func() {
			s.sendFile(fileName, fileSize, fileInfo)
			fileSentChan <- true
		}()
	})

	// 设置ICE连接状态变化
	iceConnected := make(chan bool, 1)
	iceFailed := make(chan bool, 1)
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if s.debug {
			fmt.Printf("ICE连接状态: %s\n", state.String())
		}
		switch state {
		case webrtc.ICEConnectionStateConnected:
			if s.debug {
				fmt.Println("ICE连接已建立!")
			}
			select {
			case iceConnected <- true:
			default:
			}
		case webrtc.ICEConnectionStateFailed, webrtc.ICEConnectionStateDisconnected, webrtc.ICEConnectionStateClosed:
			if s.debug {
				fmt.Printf("ICE连接失败: %s\n", state.String())
			}
			select {
			case iceFailed <- true:
			default:
			}
		}
	})

	// 监听ICE候选者
	iceGatheringComplete := make(chan bool, 1)
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			// ICE候选者收集完成
			if s.debug {
				fmt.Println("ICE候选者收集完成")
			}
			select {
			case iceGatheringComplete <- true:
			default:
			}
		} else {
			if s.debug {
				fmt.Println("ICE候选者: %s\n", candidate.String())
			}
		}
	})

	// 创建Offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("创建Offer失败: %w", err)
	}

	// 设置LocalDescription（这会触发ICE候选者收集）
	if err = pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("设置LocalDescription失败: %w", err)
	}

	// 等待ICE候选者收集完成
	if s.debug {
		fmt.Println("等待ICE候选者收集...")
	}
	select {
	case <-iceGatheringComplete:
		// 重新获取更新后的SDP（包含ICE候选者）
		offer = *pc.LocalDescription()
		if s.debug {
			fmt.Println("ICE候选者已收集完成")
		}
	case <-time.After(10 * time.Second):
		fmt.Println("警告: ICE候选者收集超时，继续使用当前SDP")
	}

	// 将SDP编码为base64
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return fmt.Errorf("序列化Offer失败: %w", err)
	}
	offerB64 := base64.StdEncoding.EncodeToString(offerJSON)

	// 打印SDP信息（仅在debug模式下）
	if s.debug {
		fmt.Println("\n" + strings.Repeat("=", 70))
		fmt.Println("SDP Offer 信息:")
		fmt.Println(strings.Repeat("=", 70))
		fmt.Printf("类型: %s\n", offer.Type)
		fmt.Printf("SDP内容:\n%s\n", offer.SDP)
		fmt.Println(strings.Repeat("=", 70))
	}

	// 使用默认信令服务器（如果未指定）
	signalingURL := s.signalingURL
	if signalingURL == "" {
		signalingURL = getDefaultSignalingURL()
		if signalingURL != "" {
			fmt.Printf("使用默认信令服务器: %s\n", signalingURL)
		}
	}

	// 连接信令服务器
	var signalingClient *SignalingClient
	if signalingURL != "" {
		fmt.Println("正在连接信令服务器...")
		signalingClient, err = NewSignalingClient(signalingURL)
		if err != nil {
			return fmt.Errorf("连接信令服务器失败: %w", err)
		}
		defer signalingClient.Close()

		// 创建房间
		roomID := s.roomID
		if roomID == "" {
			roomID = s.fileID // 使用文件ID作为房间ID
		}

		if s.debug {
			fmt.Printf("创建房间: %s\n", roomID)
		}
		signalingClient.Send(&Message{
			Type: "create_room",
			RoomID: roomID,
		})

		// 等待房间创建确认
		msg, err := signalingClient.Receive(5 * time.Second)
		if err != nil {
			return fmt.Errorf("等待房间创建失败: %w", err)
		}

		if msg.Type == "error" {
			return fmt.Errorf("创建房间失败: %s", msg.Error)
		}

		if msg.Type != "room_created" {
			return fmt.Errorf("意外的消息类型: %s", msg.Type)
		}

		fmt.Printf("房间已创建: %s\n", roomID)
		fmt.Printf("文件编号: %s\n", s.fileID)
		fmt.Println("\n等待接收端加入...")

		// 等待接收端加入（收到peer_joined消息）
		offerSent := false
		for !offerSent {
			msg, err := signalingClient.Receive(5 * time.Minute)
			if err != nil {
				return fmt.Errorf("等待接收端加入失败: %w", err)
			}

			if msg.Type == "peer_joined" {
				fmt.Println("接收端已加入，发送Offer...")
				// 发送Offer
				signalingClient.Send(&Message{
					Type: "offer",
					RoomID: roomID,
					FileID: s.fileID,
					SDP: offerB64,
				})
				offerSent = true
				fmt.Println("Offer已发送，等待Answer...")
			} else if msg.Type == "error" {
				return fmt.Errorf("信令服务器错误: %s", msg.Error)
			}
		}

		// 等待Answer
		for {
			msg, err := signalingClient.Receive(5 * time.Minute)
			if err != nil {
				return fmt.Errorf("接收Answer失败: %w", err)
			}

			if msg.Type == "answer" {
				// 解码Answer
				answerJSON, err := base64.StdEncoding.DecodeString(msg.SDP)
				if err != nil {
					return fmt.Errorf("解码Answer失败: %w", err)
				}

				var answer webrtc.SessionDescription
				if err = json.Unmarshal(answerJSON, &answer); err != nil {
					return fmt.Errorf("解析Answer失败: %w", err)
				}

				// 打印SDP Answer信息（仅在debug模式下）
				if s.debug {
					fmt.Println("\n" + strings.Repeat("=", 70))
					fmt.Println("SDP Answer 信息:")
					fmt.Println(strings.Repeat("=", 70))
					fmt.Printf("类型: %s\n", answer.Type)
					fmt.Printf("SDP内容:\n%s\n", answer.SDP)
					fmt.Println(strings.Repeat("=", 70))
				}

				// 设置RemoteDescription
				if err = pc.SetRemoteDescription(answer); err != nil {
					return fmt.Errorf("设置RemoteDescription失败: %w", err)
				}

				fmt.Println("Answer已设置，等待连接建立...")
				break
			} else if msg.Type == "error" {
				return fmt.Errorf("信令服务器错误: %s", msg.Error)
			}
		}
	} else {
		// 无信令服务器，使用手动输入方式
		fmt.Println("\n" + strings.Repeat("=", 70))
		fmt.Println("WebRTC连接已创建!")
		fmt.Println(strings.Repeat("=", 70))
		fmt.Printf("文件编号: %s\n", s.fileID)
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("请将以下信息发送给接收端:")
		fmt.Println(strings.Repeat("-", 70))
		fmt.Printf("文件编号: %s\n", s.fileID)
		fmt.Printf("SDP Offer (base64):\n%s\n", offerB64)
		fmt.Println(strings.Repeat("=", 70))
		fmt.Println("\n等待接收端Answer...")
		fmt.Print("\n请输入Answer (base64): ")

		var answerB64 string
		fmt.Scanln(&answerB64)
		
		if answerB64 == "" {
			return fmt.Errorf("未收到Answer")
		}

		answerJSON, err := base64.StdEncoding.DecodeString(answerB64)
		if err != nil {
			return fmt.Errorf("解码Answer失败: %w", err)
		}

		var answer webrtc.SessionDescription
		if err = json.Unmarshal(answerJSON, &answer); err != nil {
			return fmt.Errorf("解析Answer失败: %w", err)
		}

		if err = pc.SetRemoteDescription(answer); err != nil {
			return fmt.Errorf("设置RemoteDescription失败: %w", err)
		}

		fmt.Println("Answer已设置，等待连接建立...")
	}

	// 等待ICE连接建立
	fmt.Println("等待ICE连接建立...")
	iceTimeout := time.After(60 * time.Second)
	select {
	case <-iceConnected:
		fmt.Println("ICE连接已建立，等待DataChannel打开...")
	case <-iceFailed:
		return fmt.Errorf("ICE连接失败，无法建立P2P连接")
	case <-iceTimeout:
		return fmt.Errorf("等待ICE连接超时")
	}

	// 等待DataChannel打开
	dcOpenTimeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	dcOpened := false
	for !dcOpened {
		select {
		case <-dcOpenTimeout:
			return fmt.Errorf("等待DataChannel打开超时（ICE连接可能未完全建立）")
		case <-iceFailed:
			return fmt.Errorf("ICE连接失败，DataChannel无法打开")
		case <-ticker.C:
			if dc.ReadyState() == webrtc.DataChannelStateOpen {
				dcOpened = true
			}
		}
	}

	// 等待文件传输完成
	fmt.Println("等待文件传输完成...")
	select {
	case <-fileSentChan:
		fmt.Println("文件已发送完成，等待接收端确认...")
		// 等待接收端确认接收完成，或者超时
		select {
		case <-fileReceivedAck:
			fmt.Println("接收端已确认，关闭连接，可以关闭窗口了（按Ctrl+C退出）")
		case <-time.After(5 * time.Minute):
			fmt.Println("警告: 等待接收端确认超时，但文件已发送完成")
		}
		return nil
	case <-time.After(30 * time.Minute):
		return fmt.Errorf("文件传输超时")
	}
}

// sendFile 发送文件
func (s *WebRTCSender) sendFile(fileName string, fileSize int64, fileInfo os.FileInfo) {
	// 打开文件
	file, err := os.Open(s.filePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return
	}
	defer file.Close()

	// 发送文件元数据
	metadata := FileMetadata{
		FileName: fileName,
		FileSize: fileSize,
	}
	metadataJSON, _ := json.Marshal(metadata)
	metadataLen := uint32(len(metadataJSON))

	// 发送元数据长度和元数据
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, metadataLen)
	s.dc.Send(lenBuf)
	s.dc.Send(metadataJSON)

	fmt.Println("元数据已发送，开始传输文件数据...")
	fmt.Println()

	// 发送文件数据
	// WebRTC DataChannel最大消息大小为65536字节，使用32KB缓冲区确保不超过限制
	const maxChunkSize = 32 * 1024 // 32KB
	buffer := make([]byte, maxChunkSize)
	var totalSent int64
	startTime := time.Now()

	for {
		n, err := file.Read(buffer)
		if n > 0 {
			// 确保不超过最大消息大小限制
			chunkSize := n
			if chunkSize > maxChunkSize {
				chunkSize = maxChunkSize
			}
			
			// 如果读取的数据超过限制，分块发送
			offset := 0
			for offset < n {
				chunk := chunkSize
				if offset+chunk > n {
					chunk = n - offset
				}
				
				// 发送数据块
				if sendErr := s.dc.Send(buffer[offset : offset+chunk]); sendErr != nil {
					fmt.Printf("\n发送数据失败: %v\n", sendErr)
					return
				}
				offset += chunk
				totalSent += int64(chunk)
				
				// 显示进度
				elapsed := time.Since(startTime).Seconds()
				if elapsed > 0 {
					progress := float64(totalSent) / float64(fileSize) * 100
					speed := float64(totalSent) / elapsed / 1024 / 1024 // MB/s
					fmt.Printf("\r进度: %.2f%% | 已传输: %d / %d 字节 | 速度: %.2f MB/s", 
						progress, totalSent, fileSize, speed)
				}
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("\n读取文件失败: %v\n", err)
			return
		}
	}

	elapsed := time.Since(startTime).Seconds()
	fmt.Printf("\n\n传输完成!\n")
	fmt.Printf("总大小: %d 字节 (%.2f MB)\n", totalSent, float64(totalSent)/1024/1024)
	fmt.Printf("耗时: %.2f 秒\n", elapsed)
	if elapsed > 0 {
		fmt.Printf("平均速度: %.2f MB/s\n", float64(totalSent)/elapsed/1024/1024)
	}
}

// getDefaultICEServers 获取默认ICE服务器配置
// 如果用户指定了stunServer或turnServer，则使用用户指定的；否则使用默认配置
func getDefaultICEServers(stunServer, turnServer string, debug bool) []webrtc.ICEServer {
	iceServers := []webrtc.ICEServer{}

	// 如果用户指定了STUN服务器，使用用户指定的
	if stunServer != "" {
		stunURL := stunServer
		if !strings.HasPrefix(stunURL, "stun:") {
			stunURL = "stun:" + stunURL
		}
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: []string{stunURL},
		})
		if debug {
			fmt.Printf("STUN服务器: %s\n", stunURL)
		}
	} else {
		// 使用默认STUN服务器
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: []string{"stun:175.24.2.28:3478"},
		})
		if debug {
			fmt.Printf("使用默认STUN服务器: stun:175.24.2.28:3478\n")
		}
	}

	// 如果用户指定了TURN服务器，使用用户指定的
	if turnServer != "" {
		turnURL := turnServer
		if !strings.HasPrefix(turnURL, "turn:") {
			turnURL = "turn:" + turnURL
		}
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: []string{turnURL},
		})
		if debug {
			fmt.Printf("TURN服务器: %s\n", turnURL)
		}
	} else {
		// 使用默认TURN服务器
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: []string{
				"turn:175.24.2.28:3478?transport=udp",
				"turn:175.24.2.28:3478?transport=tcp",
			},
			Username:   "demo",
			Credential: "demo123",
		})
		if debug {
			fmt.Printf("使用默认TURN服务器: turn:175.24.2.28:3478 (username: demo)\n")
		}
	}

	return iceServers
}

// getDefaultSignalingURL 获取默认信令服务器URL
func getDefaultSignalingURL() string {
	return "ws://175.24.2.28:37851/ws"
}

