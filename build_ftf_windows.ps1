# PowerShell脚本：编译Windows版本的文件传输工具

Write-Host "正在编译Windows版本的文件传输工具..." -ForegroundColor Green

$env:GOOS = "windows"
$env:GOARCH = "amd64"

go build -trimpath -ldflags="-s -w" -o ftf.exe

if ($LASTEXITCODE -eq 0) {
    Write-Host "编译成功！Windows版本已生成: ftf.exe" -ForegroundColor Green
} else {
    Write-Host "编译失败！" -ForegroundColor Red
    exit 1
}

