package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HTTPSender HTTP文件服务器
type HTTPSender struct {
	filePath string
	port     int
	server   *http.Server
}

// NewHTTPSender 创建HTTP发送端
func NewHTTPSender(filePath string, port int) *HTTPSender {
	return &HTTPSender{
		filePath: filePath,
		port:     port,
	}
}

// Start 启动HTTP文件服务器
func (s *HTTPSender) Start() error {
	// 检查文件是否存在
	fileInfo, err := os.Stat(s.filePath)
	if err != nil {
		return fmt.Errorf("文件不存在: %w", err)
	}

	fileName := filepath.Base(s.filePath)
	fileSize := fileInfo.Size()

	fmt.Printf("文件: %s\n", fileName)
	fmt.Printf("大小: %d 字节 (%.2f MB)\n", fileSize, float64(fileSize)/1024/1024)

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

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", actualPort),
		Handler: mux,
	}

	// 生成下载命令
	downloadURL := fmt.Sprintf("http://%s:%d/download", localIP, actualPort)
	downloadCmd := fmt.Sprintf("ftf.exe receive \"%s\" \"%s\"", downloadURL, fileName)

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("文件服务器已启动!")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("下载地址: %s\n", downloadURL)
	fmt.Println(strings.Repeat("-", 70))
	fmt.Println("复制以下命令到另一台电脑执行:")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%s\n", downloadCmd)
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("\n服务器运行中，按 Ctrl+C 停止...\n\n")

	// 启动服务器
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("服务器错误: %w", err)
	}

	return nil
}

// getLocalIP 获取本机局域网IP地址
func getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

