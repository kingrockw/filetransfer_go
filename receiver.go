package main

import (
	"fmt"
	"net/url"
	"strings"
)

// AutoReceiver 自动判断接收模式
type AutoReceiver struct {
	address  string
	savePath string
	// WebRTC参数
	stunServer   string
	turnServer   string
	signalingURL string
	roomID       string
}

// NewAutoReceiver 创建自动接收器
func NewAutoReceiver(address, savePath, stunServer, turnServer, signalingURL, roomID string) *AutoReceiver {
	return &AutoReceiver{
		address:      address,
		savePath:     savePath,
		stunServer:   stunServer,
		turnServer:   turnServer,
		signalingURL: signalingURL,
		roomID:       roomID,
	}
}

// Start 开始接收文件（自动判断模式）
func (r *AutoReceiver) Start() error {
	// 判断是HTTP还是WebRTC
	if r.isHTTPAddress(r.address) {
		// HTTP模式
		fmt.Println("检测到HTTP地址，使用HTTP模式下载...")
		receiver := NewHTTPReceiver(r.address, r.savePath)
		return receiver.Start()
	} else {
		// WebRTC模式（文件编号或SDP）
		fmt.Println("检测到WebRTC模式，使用WebRTC接收...")
		
		// 解析地址：可能是文件编号，也可能是"文件编号|SDP Offer"格式
		parts := strings.Split(r.address, "|")
		fileID := parts[0]
		sdpOffer := ""
		if len(parts) > 1 {
			sdpOffer = parts[1]
		}
		
		// 如果savePath为空，使用默认目录
		if r.savePath == "" || r.savePath == "." {
			r.savePath = "D:\\ft_download"
		}
		
		receiver := NewWebRTCReceiver(fileID, sdpOffer, r.savePath, r.stunServer, r.turnServer, r.signalingURL, r.roomID, false)
		return receiver.Start()
	}
}

// isHTTPAddress 判断是否是HTTP地址
func (r *AutoReceiver) isHTTPAddress(addr string) bool {
	// 检查是否是URL格式
	addrLower := strings.ToLower(addr)
	if strings.HasPrefix(addrLower, "http://") || strings.HasPrefix(addrLower, "https://") {
		return true
	}
	
	// 尝试解析为URL
	if u, err := url.Parse(addr); err == nil {
		if u.Scheme == "http" || u.Scheme == "https" {
			return true
		}
	}
	
	// 如果包含://，但不是http/https，可能是其他协议
	if strings.Contains(addr, "://") {
		return false
	}
	
	// 如果看起来像文件编号（16位hex字符串），则不是HTTP
	if len(addr) == 16 {
		// 检查是否是hex字符串
		isHex := true
		for _, c := range addr {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				isHex = false
				break
			}
		}
		if isHex {
			return false // 是文件编号，使用WebRTC
		}
	}
	
	// 如果包含斜杠或点，可能是URL的一部分
	if strings.Contains(addr, "/") || strings.Contains(addr, ".") {
		// 可能是IP地址或域名，尝试作为HTTP处理
		return true
	}
	
	// 默认情况下，如果不是明确的HTTP URL，尝试作为文件编号处理（WebRTC）
	return false
}

