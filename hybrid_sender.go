package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// HybridSender 混合发送器，同时支持HTTP和WebRTC
type HybridSender struct {
	filePath     string
	port         int
	stunServer   string
	turnServer   string
	signalingURL string
	roomID       string
	debug        bool
	httpServer   *http.Server
	webrtcSender *WebRTCSender
	wg           sync.WaitGroup
}

// NewHybridSender 创建混合发送器
func NewHybridSender(filePath string, port int, stunServer, turnServer, signalingURL, roomID string) *HybridSender {
	return &HybridSender{
		filePath:     filePath,
		port:         port,
		stunServer:   stunServer,
		turnServer:   turnServer,
		signalingURL: signalingURL,
		roomID:       roomID,
	}
}

// Start 启动混合发送器（同时启动HTTP和WebRTC）
func (s *HybridSender) Start() error {
	// 检查文件是否存在
	fileInfo, err := os.Stat(s.filePath)
	if err != nil {
		return fmt.Errorf("文件不存在: %w", err)
	}

	fileName := filepath.Base(s.filePath)
	fileSize := fileInfo.Size()

	// 生成随机文件ID（用于WebRTC）
	fileID := generateFileID()

	fmt.Println("=== 文件传输服务 ===")
	fmt.Printf("文件: %s\n", fileName)
	fmt.Printf("大小: %d 字节 (%.2f MB)\n", fileSize, float64(fileSize)/1024/1024)
	fmt.Printf("文件编号: %s\n", fileID)

	// 获取本机IP地址
	localIP, err := getLocalIP()
	if err != nil {
		return fmt.Errorf("获取本机IP失败: %w", err)
	}

	// 如果未指定端口，使用随机端口
	actualPort := s.port
	if actualPort == 0 {
		// 使用随机端口
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			return fmt.Errorf("监听端口失败: %w", err)
		}
		actualPort = listener.Addr().(*net.TCPAddr).Port
		listener.Close()
	}

	// 启动HTTP服务器（在goroutine中）
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startHTTPServer(fileName, fileSize, fileInfo, localIP, actualPort); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP服务器错误: %v\n", err)
		}
	}()

	// 启动WebRTC发送端（在goroutine中）
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.webrtcSender = NewWebRTCSender(s.filePath, s.stunServer, s.turnServer, s.signalingURL, s.roomID)
		// 设置文件ID和debug标志
		s.webrtcSender.fileID = fileID
		s.webrtcSender.debug = s.debug
		if err := s.webrtcSender.Start(); err != nil {
			fmt.Printf("WebRTC发送错误: %v\n", err)
		}
	}()

	// 显示连接信息
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("文件传输服务已启动!")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("\n【局域网下载 - HTTP模式】")
	fmt.Printf("内网地址: http://%s:%d/download\n", localIP, actualPort)
	fmt.Printf("下载命令: ftf.exe receive \"http://%s:%d/download\"\n", localIP, actualPort)
	fmt.Println("\n【跨网络传输 - WebRTC模式】")
	fmt.Printf("文件编号: %s\n", fileID)
	fmt.Printf("接收命令: ftf.exe receive \"%s\"\n", fileID)
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("\n服务运行中，按 Ctrl+C 停止...\n\n")

	// 等待所有goroutine完成
	s.wg.Wait()
	return nil
}

// startHTTPServer 启动HTTP服务器
func (s *HybridSender) startHTTPServer(fileName string, fileSize int64, fileInfo os.FileInfo, localIP string, port int) error {
	// 创建HTTP服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		// 设置响应头
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))

		// 打开文件
		file, err := os.Open(s.filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer file.Close()

		// 发送文件
		http.ServeContent(w, r, fileName, fileInfo.ModTime(), file)
	})

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// 启动服务器
	return s.httpServer.ListenAndServe()
}

// Stop 停止混合发送器
func (s *HybridSender) Stop() error {
	// 停止HTTP服务器
	if s.httpServer != nil {
		ctx := context.Background()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return err
		}
	}

	// WebRTC发送端会在连接关闭时自动停止
	return nil
}

