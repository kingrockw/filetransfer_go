package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HTTPReceiver HTTP文件下载客户端
type HTTPReceiver struct {
	downloadURL string
	savePath    string
}

// NewHTTPReceiver 创建HTTP接收端
func NewHTTPReceiver(downloadURL, savePath string) *HTTPReceiver {
	return &HTTPReceiver{
		downloadURL: downloadURL,
		savePath:    savePath,
	}
}

// Start 开始下载文件
func (r *HTTPReceiver) Start() error {
	fmt.Println("=== 开始下载文件 ===")
	fmt.Printf("下载地址: %s\n", r.downloadURL)
	fmt.Printf("保存路径: %s\n", r.savePath)

	// 创建HTTP请求
	client := &http.Client{
		Timeout: 30 * time.Minute,
	}

	resp, err := client.Get(r.downloadURL)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误: %d %s", resp.StatusCode, resp.Status)
	}

	// 获取文件大小
	fileSize := resp.ContentLength
	if fileSize <= 0 {
		fileSize = 0
	}

	// 确定保存路径
	savePath := r.savePath
	if savePath == "" || savePath == "." {
		// 从Content-Disposition获取文件名
		contentDisposition := resp.Header.Get("Content-Disposition")
		fileName := "download"
		if contentDisposition != "" {
			// 简单解析 filename="xxx"
			if idx := strings.Index(contentDisposition, "filename=\""); idx >= 0 {
				start := idx + len("filename=\"")
				if end := strings.Index(contentDisposition[start:], "\""); end >= 0 {
					fileName = contentDisposition[start : start+end]
				}
			}
		}
		savePath = fileName
	}

	// 如果savePath是目录，使用URL中的文件名
	if info, err := os.Stat(savePath); err == nil && info.IsDir() {
		fileName := filepath.Base(r.downloadURL)
		if fileName == "download" {
			// 尝试从Content-Disposition获取
			contentDisposition := resp.Header.Get("Content-Disposition")
			if contentDisposition != "" {
				if idx := strings.Index(contentDisposition, "filename=\""); idx >= 0 {
					start := idx + len("filename=\"")
					if end := strings.Index(contentDisposition[start:], "\""); end >= 0 {
						fileName = contentDisposition[start : start+end]
					}
				}
			}
		}
		savePath = filepath.Join(savePath, fileName)
	} else if err != nil && os.IsNotExist(err) {
		// savePath可能是目录但不存在，尝试创建
		dir := filepath.Dir(savePath)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err == nil {
				// 如果创建成功，说明savePath是目录，需要添加文件名
				fileName := filepath.Base(r.downloadURL)
				if fileName == "download" {
					contentDisposition := resp.Header.Get("Content-Disposition")
					if contentDisposition != "" {
						if idx := strings.Index(contentDisposition, "filename=\""); idx >= 0 {
							start := idx + len("filename=\"")
							if end := strings.Index(contentDisposition[start:], "\""); end >= 0 {
								fileName = contentDisposition[start : start+end]
							}
						}
					}
				}
				savePath = filepath.Join(savePath, fileName)
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

	// 创建文件
	file, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	fmt.Printf("保存到: %s\n", savePath)
	if fileSize > 0 {
		fmt.Printf("文件大小: %d 字节 (%.2f MB)\n", fileSize, float64(fileSize)/1024/1024)
	}
	fmt.Println("开始下载...")

	// 下载文件
	buffer := make([]byte, 64*1024) // 64KB
	var totalReceived int64
	startTime := time.Now()

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			written, writeErr := file.Write(buffer[:n])
			if writeErr != nil {
				return fmt.Errorf("写入文件失败: %w", writeErr)
			}
			totalReceived += int64(written)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取数据失败: %w", err)
		}

		// 显示进度
		if fileSize > 0 {
			progress := float64(totalReceived) / float64(fileSize) * 100
			elapsed := time.Since(startTime).Seconds()
			if elapsed > 0 {
				speed := float64(totalReceived) / elapsed / 1024 / 1024 // MB/s
				fmt.Printf("\r进度: %.2f%% (%.2f MB/s)", progress, speed)
			}
		} else {
			elapsed := time.Since(startTime).Seconds()
			if elapsed > 0 {
				speed := float64(totalReceived) / elapsed / 1024 / 1024 // MB/s
				fmt.Printf("\r已下载: %.2f MB (%.2f MB/s)", float64(totalReceived)/1024/1024, speed)
			}
		}
	}

	elapsed := time.Since(startTime).Seconds()
	
	// 获取文件的绝对路径
	absPath, _ := filepath.Abs(savePath)
	
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✓ 下载完成!")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("文件保存路径: %s\n", absPath)
	fmt.Printf("总大小: %d 字节 (%.2f MB)\n", totalReceived, float64(totalReceived)/1024/1024)
	fmt.Printf("耗时: %.2f 秒\n", elapsed)
	if elapsed > 0 {
		fmt.Printf("平均速度: %.2f MB/s\n", float64(totalReceived)/elapsed/1024/1024)
	}
	fmt.Println(strings.Repeat("=", 70))

	return nil
}

