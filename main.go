package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "filetransfer",
		Short: "文件传输工具",
		Long:  "文件传输工具，支持HTTP服务器模式和WebRTC P2P模式",
		Version: version,
	}

	// 发送命令
	var sendCmd = &cobra.Command{
		Use:   "send [文件路径]",
		Short: "发送文件",
		Long:  "发送文件，默认同时支持HTTP（局域网）和WebRTC（跨网络）两种模式",
		Args:  cobra.ExactArgs(1),
		Run:   runSend,
	}

	sendCmd.Flags().IntP("port", "p", 0, "HTTP服务器端口（默认随机端口）")
	sendCmd.Flags().Bool("webrtc", false, "仅使用WebRTC P2P模式（不启动HTTP服务器）")
	sendCmd.Flags().Bool("http", false, "仅使用HTTP服务器模式（不启动WebRTC）")
	sendCmd.Flags().Bool("debug", false, "显示调试信息（包括SDP详情）")
	sendCmd.Flags().String("stun", "", "STUN服务器地址（格式: host:port，默认: stun:175.24.2.28:3478）")
	sendCmd.Flags().String("turn", "", "TURN服务器地址（格式: host:port，默认: turn:175.24.2.28:3478）")
	sendCmd.Flags().String("signaling", "", "信令服务器地址（格式: ws://host:port/ws，默认: ws://175.24.2.28:37851/ws）")
	sendCmd.Flags().String("room", "", "房间ID（WebRTC模式，默认使用文件编号）")

	// 接收命令（自动判断HTTP或WebRTC）
	var receiveCmd = &cobra.Command{
		Use:   "receive [地址/文件编号] [保存路径]",
		Short: "接收文件（自动判断模式）",
		Long:  "接收文件，自动判断是HTTP地址还是WebRTC文件编号。HTTP地址格式: http://ip:port/download，WebRTC格式: 文件编号",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runReceive,
	}

	receiveCmd.Flags().String("stun", "", "STUN服务器地址（格式: host:port，默认: stun:175.24.2.28:3478）")
	receiveCmd.Flags().String("turn", "", "TURN服务器地址（格式: host:port，默认: turn:175.24.2.28:3478）")
	receiveCmd.Flags().String("signaling", "", "信令服务器地址（格式: ws://host:port/ws，默认: ws://175.24.2.28:37851/ws）")
	receiveCmd.Flags().String("room", "", "房间ID（WebRTC模式，默认使用文件编号）")

	rootCmd.AddCommand(sendCmd, receiveCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func runSend(cmd *cobra.Command, args []string) {
	filePath := args[0]
	port, _ := cmd.Flags().GetInt("port")
	useWebRTCOnly, _ := cmd.Flags().GetBool("webrtc")
	useHTTPOnly, _ := cmd.Flags().GetBool("http")
	debug, _ := cmd.Flags().GetBool("debug")
	stunServer, _ := cmd.Flags().GetString("stun")
	turnServer, _ := cmd.Flags().GetString("turn")
	signalingURL, _ := cmd.Flags().GetString("signaling")
	roomID, _ := cmd.Flags().GetString("room")

	if useWebRTCOnly {
		// 仅使用WebRTC模式
		sender := NewWebRTCSender(filePath, stunServer, turnServer, signalingURL, roomID)
		sender.debug = debug
		if err := sender.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "发送失败: %v\n", err)
			os.Exit(1)
		}
	} else if useHTTPOnly {
		// 仅使用HTTP模式（port为0时使用随机端口）
		sender := NewHTTPSender(filePath, port)
		if err := sender.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "发送失败: %v\n", err)
			os.Exit(1)
		}
	} else {
		// 混合模式：同时启动HTTP和WebRTC（port为0时使用随机端口）
		sender := NewHybridSender(filePath, port, stunServer, turnServer, signalingURL, roomID)
		sender.debug = debug
		if err := sender.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "发送失败: %v\n", err)
			os.Exit(1)
		}
	}
}

func runReceive(cmd *cobra.Command, args []string) {
	address := args[0]
	savePath := ""
	if len(args) > 1 {
		savePath = args[1]
	}
	if savePath == "" {
		savePath = "D:\\ft_download"
	}

	stunServer, _ := cmd.Flags().GetString("stun")
	turnServer, _ := cmd.Flags().GetString("turn")
	signalingURL, _ := cmd.Flags().GetString("signaling")
	roomID, _ := cmd.Flags().GetString("room")

	receiver := NewAutoReceiver(address, savePath, stunServer, turnServer, signalingURL, roomID)
	if err := receiver.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "接收失败: %v\n", err)
		os.Exit(1)
	}
}
