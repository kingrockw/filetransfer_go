# WebRTC 信令服务器使用指南

## 概述

信令服务器用于自动交换WebRTC的SDP（Session Description Protocol）信息，实现P2P文件传输的自动连接。

## 编译信令服务器

### 方法1：在cmd/signaling目录下编译（推荐）

```bash
# 进入信令服务器目录
cd cmd/signaling

# 编译（Windows）
go build -trimpath -ldflags="-s -w" -o signaling-server.exe

# 编译（Linux）
go build -trimpath -ldflags="-s -w" -o signaling-server

# 编译（macOS）
go build -trimpath -ldflags="-s -w" -o signaling-server
```

### 方法2：从项目根目录编译

```bash
# 从项目根目录编译
go build -trimpath -ldflags="-s -w" -o signaling-server.exe ./cmd/signaling

# 或指定输出目录
go build -trimpath -ldflags="-s -w" -o bin/signaling-server.exe ./cmd/signaling
```

### 方法3：交叉编译（跨平台）

```bash
# Windows (64位)
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o signaling-server.exe ./cmd/signaling

# Linux (64位)
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o signaling-server ./cmd/signaling

# macOS (64位)
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o signaling-server ./cmd/signaling

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o signaling-server ./cmd/signaling
```

## 启动信令服务器

```bash
# 启动服务器（默认端口37851）
signaling-server.exe

# 或指定端口
signaling-server.exe -port 9000
```

## 使用方式

### 方式1：使用信令服务器（推荐）

**步骤1：启动信令服务器**

```bash
signaling-server.exe
# 默认端口37851，或指定端口: signaling-server.exe -port 37851
```

**步骤2：发送端创建房间并发送文件**

```bash
# 使用默认信令服务器（无需指定--signaling参数）
ftf.exe send "file.txt" --webrtc

# 或手动指定信令服务器
ftf.exe send "file.txt" --webrtc --signaling "ws://175.24.2.28:37851/ws"

# 显示调试信息（包括SDP详情）
ftf.exe send "file.txt" --webrtc --debug
```

发送端会：

- 自动创建房间（使用文件编号作为房间ID）
- 生成SDP Offer
- 通过信令服务器发送Offer
- 等待接收端加入并接收Answer

**步骤3：接收端加入房间并接收文件**

```bash
# 使用默认信令服务器（只需指定文件编号）
ftf.exe receive <文件编号>

# 或手动指定信令服务器
ftf.exe receive <文件编号> --signaling "ws://175.24.2.28:37851/ws"
```

接收端会：

- 自动加入房间
- 接收Offer
- 生成并发送Answer
- 开始接收文件

### 方式2：手动交换SDP（不使用信令服务器）

如果不想使用信令服务器，可以手动复制粘贴SDP：

**发送端：**

```bash
ftf.exe send "file.txt" --webrtc --debug
# 复制显示的SDP Offer和文件编号（需要--debug参数才会显示SDP）
```

**接收端：**

```bash
ftf.exe receive <文件编号>|<SDP Offer>
# 复制显示的SDP Answer并粘贴到发送端
```

## 房间机制

- **房间ID**：默认使用文件编号作为房间ID
- **自定义房间ID**：使用 `--room` 参数指定
- **房间生命周期**：当所有客户端离开后，房间自动删除

## 消息协议

信令服务器使用WebSocket协议，消息格式为JSON：

```json
{
  "type": "create_room|join_room|offer|answer|error",
  "room_id": "房间ID",
  "file_id": "文件编号",
  "sdp": "SDP内容（base64编码）",
  "error": "错误信息"
}
```

## 注意事项

1. 信令服务器不需要认证，任何客户端都可以创建或加入房间
2. 建议在内网或受信任的网络环境中使用
3. 信令服务器只负责交换SDP，不传输实际文件数据
4. 文件传输通过WebRTC P2P直连，不经过信令服务器


# 启动服务

sudo systemctl start signaling-server

# 停止服务

sudo systemctl stop signaling-server

# 重启服务

sudo systemctl restart signaling-server

# 查看服务状态

sudo systemctl status signaling-server

# 查看实时日志

sudo journalctl -u signaling-server -f

# 查看最近100行日志

sudo journalctl -u signaling-server -n 100

# 设置开机自启

sudo systemctl enable signaling-server

# 取消开机自启

sudo systemctl disable signaling-server
