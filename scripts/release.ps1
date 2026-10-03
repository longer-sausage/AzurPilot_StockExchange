#requires -Version 7.0
# 在 Windows / PowerShell 7 构建 Debian 发布包，无需 WSL 或 C 编译器。
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Invoke-CheckedCommand {
    param([string]$Command, [string[]]$Arguments)
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command 执行失败（退出码 $LASTEXITCODE），发布已停止。"
    }
}

$rootDir = Split-Path -Parent $PSScriptRoot
$npmCommand = if ($IsWindows) { 'npm.cmd' } else { 'npm' }
foreach ($command in @($npmCommand, 'go', 'tar')) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw "未找到 $command，请先安装 Go >= 1.24、Node.js >= 18，并确保 tar 可用。"
    }
}

$savedEnvironment = @{}
foreach ($name in @('CGO_ENABLED', 'GOOS', 'GOARCH')) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$packageDir = [IO.Path]::GetFullPath((Join-Path $tempRoot ('mmex-package-' + [guid]::NewGuid().ToString('N'))))

Push-Location $rootDir
try {
    Invoke-CheckedCommand $npmCommand @('ci', '--prefix', 'frontend', '--no-audit', '--no-fund')
    Invoke-CheckedCommand $npmCommand @('run', 'build', '--prefix', 'frontend')

    # 测试在本机运行，避免继承 Linux 目标后尝试执行 Linux 测试程序。
    $env:CGO_ENABLED = '0'
    [Environment]::SetEnvironmentVariable('GOOS', $null, 'Process')
    [Environment]::SetEnvironmentVariable('GOARCH', $null, 'Process')
    Invoke-CheckedCommand 'go' @('test', './...')

    $binDir = Join-Path $rootDir 'bin'
    $releaseDir = Join-Path $rootDir 'releases'
    New-Item -ItemType Directory -Force -Path $binDir, $releaseDir | Out-Null
    $env:GOOS = 'linux'
    foreach ($arch in @('amd64', 'arm64')) {
        $env:GOARCH = $arch
        Write-Host "正在构建 linux/$arch..."
        Invoke-CheckedCommand 'go' @('build', '-trimpath', '-ldflags=-s -w', '-o', (Join-Path $binDir "exchange-linux-$arch"), './cmd/exchange')
    }

    $packageBin = Join-Path $packageDir 'bin'
    $packageFrontend = Join-Path $packageDir 'frontend'
    New-Item -ItemType Directory -Path $packageBin, $packageFrontend | Out-Null
    foreach ($arch in @('amd64', 'arm64')) {
        Copy-Item -LiteralPath (Join-Path $binDir "exchange-linux-$arch") -Destination $packageBin
    }
    Copy-Item -LiteralPath (Join-Path $rootDir 'frontend/dist') -Destination $packageFrontend -Recurse -Force

    # 使用 Linux 相对路径、LF 和无 BOM 编码，兼容部署端 sha256sum -c。
    [string[]]$relativePaths = @(Get-ChildItem -LiteralPath $packageDir -File -Recurse -Force | ForEach-Object {
        [IO.Path]::GetRelativePath($packageDir, $_.FullName).Replace('\', '/')
    })
    [Array]::Sort($relativePaths, [StringComparer]::Ordinal)
    $checksums = foreach ($relativePath in $relativePaths) {
        $hash = (Get-FileHash -LiteralPath (Join-Path $packageDir $relativePath) -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $relativePath"
    }
    [IO.File]::WriteAllText((Join-Path $packageDir 'SHA256SUMS'), (($checksums -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))

    # 先在临时目录完成压缩，构建失败时保留已有发布包。
    $temporaryArchive = Join-Path $packageDir 'mingmiao-exchange.tar.gz'
    Invoke-CheckedCommand 'tar' @('-czf', $temporaryArchive, '-C', $packageDir, 'bin', 'frontend/dist', 'SHA256SUMS')
    $releaseArchive = Join-Path $releaseDir 'mingmiao-exchange.tar.gz'
    Move-Item -LiteralPath $temporaryArchive -Destination $releaseArchive -Force
    Write-Host '发布包已生成：'
    Write-Output $releaseArchive
}
finally {
    foreach ($name in $savedEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process')
    }
    Pop-Location
    # 递归清理前确认目标仍是本脚本在系统临时目录中创建的目录。
    $resolvedPackageDir = [IO.Path]::GetFullPath($packageDir)
    if ((Split-Path -Parent $resolvedPackageDir) -eq $tempRoot.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar) -and
        (Split-Path -Leaf $resolvedPackageDir) -match '^mmex-package-[0-9a-f]{32}$' -and
        (Test-Path -LiteralPath $resolvedPackageDir)) {
        Remove-Item -LiteralPath $resolvedPackageDir -Recurse -Force -ErrorAction Continue
    }
}
