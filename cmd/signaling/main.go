package main

import (
	"flag"
	"fmt"
	"log"
)

// 需要导入signaling_server.go中的类型和函数
// 由于它们在同一个包中，可以直接使用

func main() {
	port := flag.Int("port", 37851, "信令服务器端口")
	flag.Parse()

	fmt.Println("=== WebRTC 信令服务器 ===")
	fmt.Printf("端口: %d\n", *port)
	fmt.Printf("WebSocket端点: ws://localhost:%d/ws\n", *port)
	fmt.Println()

	server := NewSignalingServer()
	if err := server.Start(*port); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

