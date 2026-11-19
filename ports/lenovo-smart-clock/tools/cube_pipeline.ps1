param(
    [string]$Distro = 'Ubuntu',
    [Parameter(Mandatory = $true)]
    [string[]]$PipelineArgs
)
$ErrorActionPreference = 'Stop'
$scriptPath = Join-Path $PSScriptRoot 'cube_pipeline.py'
$linuxPath = (& wsl.exe --distribution $Distro -- wslpath -a $scriptPath.Replace('\', '/'))
if ($LASTEXITCODE -ne 0 -or -not $linuxPath) { throw 'Could not translate the script path in WSL.' }
Write-Host 'Running in WSL as root. All paths in PipelineArgs must be Linux/WSL paths.'
Write-Host 'For fastboot actions, USB must be attached to WSL and fastboot available there.'
& wsl.exe --distribution $Distro --user root -- python3 $linuxPath @PipelineArgs
exit $LASTEXITCODE
