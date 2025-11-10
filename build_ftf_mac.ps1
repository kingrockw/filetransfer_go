# PowerShell脚本：编译Mac版本的文件传输工具

Write-Host "正在编译Mac版本的文件传输工具..." -ForegroundColor Green

$env:GOOS = "darwin"
$env:GOARCH = "amd64"

go build -trimpath -ldflags="-s -w" -o ftf-mac

if ($LASTEXITCODE -eq 0) {
    Write-Host "编译成功！Mac版本已生成: ftf-mac" -ForegroundColor Green
} else {
    Write-Host "编译失败！" -ForegroundColor Red
    exit 1
}

