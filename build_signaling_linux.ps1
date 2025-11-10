# PowerShell脚本：编译Linux版本的信令服务器

Write-Host "正在编译Linux版本的信令服务器..." -ForegroundColor Green

$env:GOOS = "linux"
$env:GOARCH = "amd64"

go build -trimpath -ldflags="-s -w" -o signaling-server-linux ./cmd/signaling

if ($LASTEXITCODE -eq 0) {
    Write-Host "编译成功！Linux版本已生成: signaling-server-linux" -ForegroundColor Green
} else {
    Write-Host "编译失败！" -ForegroundColor Red
    exit 1
}

