$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    $unformatted = @(gofmt -l .)
    if ($unformatted.Count -gt 0) { throw "以下Go文件尚未格式化：$($unformatted -join ', ')" }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet未通过。' }
    go test ./... -count=1
    if ($LASTEXITCODE -ne 0) { throw 'Go测试未通过。' }
    foreach ($file in @('web/app.js','web/api.js','web/server.mjs')) {
        node --check $file
        if ($LASTEXITCODE -ne 0) { throw "JavaScript检查失败：$file" }
    }
    Write-Output 'Go格式、静态检查、自动测试和前端语法检查全部通过。'
} finally { Pop-Location }
