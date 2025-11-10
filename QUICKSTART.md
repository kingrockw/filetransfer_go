# 快速开始指南

## 1. 检查Go版本

```bash
go version
```

应该显示 `go version go1.21.x` 或更高版本。如果没有安装Go，请从 [golang.org](https://golang.org/dl/) 下载安装。

## 2. 下载依赖

在项目目录运行：

```bash
go mod download
```

## 3. 编译程序

```bash
# Windows PowerShell
.\build.ps1

# 或CMD
build.bat

# 或手动编译
go build -trimpath -ldflags="-s -w" -o ftf.exe
```

## 4. 运行程序

### 命令行方式

**默认模式（同时支持HTTP和WebRTC）:**

发送端：
```bash
# 使用随机端口（推荐，避免冲突）
ftf.exe send "C:\file.txt"

# 或指定端口
ftf.exe send "C:\file.txt" --port 8080

# 显示调试信息（包括SDP详情）
ftf.exe send "C:\file.txt" --debug
```

接收端（使用发送端生成的命令）：
```bash
# HTTP模式（局域网）
ftf.exe receive "http://192.168.1.100:54321/download"

# WebRTC模式（跨网络）
ftf.exe receive "abc123def4567890"
```

## 5. 局域网传输示例

假设：
- 发送端IP: `192.168.1.100`
- 接收端IP: `192.168.1.101`

**步骤1**: 在发送端（192.168.1.100）运行：
```bash
ftf.exe send "C:\file.txt"
```

程序会自动分配随机端口（例如：54321），并显示：
```
【局域网下载 - HTTP模式】
内网地址: http://192.168.1.100:54321/download
下载命令: ftf.exe receive "http://192.168.1.100:54321/download"
```

**步骤2**: 复制生成的下载命令，在接收端（192.168.1.101）运行：
```bash
ftf.exe receive "http://192.168.1.100:54321/download"
```

## 常见问题

### Q: 编译时提示找不到包？
A: 运行 `go mod download` 下载依赖

### Q: GUI界面无法显示？
A: 确保已安装所有依赖：`go mod download`，然后重新编译

### Q: 连接失败？
A: 
1. 检查防火墙是否允许端口通信（随机端口需要允许临时端口范围）
2. 确认对端IP地址正确
3. 使用发送端显示的完整地址（包含端口号）

### Q: HTTP模式 vs WebRTC模式？
A: 
- **HTTP模式**: 简单高效，适合局域网传输（推荐用于同一网络）
- **WebRTC模式**: 自动NAT穿透，适合跨网络传输和复杂网络环境



