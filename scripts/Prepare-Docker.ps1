param([switch]$Start)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$previousPath = $env:Path
Push-Location $projectRoot
try {
    # 优先使用PATH；兼容当前电脑安装在D盘的Docker Desktop。
    $dockerCommand = Get-Command docker -ErrorAction SilentlyContinue
    if ($dockerCommand) { $dockerPath = $dockerCommand.Source }
    else {
        $dockerPath = @('D:\Docker\resources\bin\docker.exe', 'C:\Program Files\Docker\Docker\resources\bin\docker.exe') |
            Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    }
    if (-not $dockerPath) { throw '未找到Docker，请安装Docker Desktop或将docker加入PATH。' }
    # Docker凭据助手也位于同一目录，子进程必须能在PATH中找到它。
    $env:Path = "$(Split-Path -Parent $dockerPath);$env:Path"
    & $dockerPath info --format '{{.OSType}}'
    if ($LASTEXITCODE -ne 0) { throw 'Docker引擎未运行，请先启动Docker Desktop。' }
    go run ./cmd/setup-docker
    if ($LASTEXITCODE -ne 0) { throw '容器配置准备失败。' }
    & $dockerPath compose config --quiet
    if ($LASTEXITCODE -ne 0) { throw 'Compose配置检查失败。' }
    if ($Start) {
        & $dockerPath compose up -d --build --wait --wait-timeout 120
        if ($LASTEXITCODE -ne 0) { throw '启动失败，请运行docker compose logs检查。' }
    }
} finally { $env:Path = $previousPath; Pop-Location }
