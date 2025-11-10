# PowerShell脚本：编译Windows版本的信令服务器

Write-Host "正在编译Windows版本的信令服务器..." -ForegroundColor Green

$env:GOOS = "windows"
$env:GOARCH = "amd64"

go build -trimpath -ldflags="-s -w" -o signaling-server-windows.exe ./cmd/signaling

if ($LASTEXITCODE -eq 0) {
    Write-Host "编译成功！Windows版本已生成: signaling-server-windows.exe" -ForegroundColor Green
} else {
    Write-Host "编译失败！" -ForegroundColor Red
    exit 1
}

