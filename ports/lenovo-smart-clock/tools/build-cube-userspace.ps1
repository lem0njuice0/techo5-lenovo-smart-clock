param(
    [string]$Repo = (Join-Path $PSScriptRoot '../../..')
)
$ErrorActionPreference = 'Stop'
$port = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$repoPath = (Resolve-Path $Repo).Path
$saved = @{}
foreach ($name in 'GOOS', 'GOARCH', 'GOARM', 'CGO_ENABLED') {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
    $env:GOOS = 'linux'
    $env:CGO_ENABLED = '0'
    $env:GOARCH = 'arm'
    $env:GOARM = '7'
    & go -C (Join-Path $repoPath 'echod') build -tags cube -o (Join-Path $repoPath 'bin/echod-cube-arm') ./cmd/echod
    if ($LASTEXITCODE -ne 0) { throw 'TECHO5 daemon build failed' }
    & go -C $repoPath build -o (Join-Path $repoPath 'bin/btbridge-cube-arm') ./cmd/btbridge
    if ($LASTEXITCODE -ne 0) { throw 'Bluetooth bridge build failed' }
    & go -C (Join-Path $port 'serial') build -o (Join-Path $port 'build/cube-serial-arm') .
    if ($LASTEXITCODE -ne 0) { throw 'runtime diagnostic build failed' }
    $env:GOARCH = 'arm64'
    Remove-Item Env:GOARM -ErrorAction SilentlyContinue
    & go -C (Join-Path $port 'rescue') build -tags cube_boot -o (Join-Path $port 'build/cube-boot-init-aarch64') .
    if ($LASTEXITCODE -ne 0) { throw 'runtime init build failed' }
} finally {
    foreach ($name in $saved.Keys) {
        [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process')
    }
}
Write-Host 'Built experimental ARM userspace; no device was accessed.'
