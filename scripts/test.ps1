$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go.exe -CommandType Application -ErrorAction Stop
$npmCommand = Get-Command npm.cmd -CommandType Application -ErrorAction Stop
$clangCommand = Get-Command clang.exe -CommandType Application -ErrorAction Stop
$clangxxCommand = Get-Command clang++.exe -CommandType Application -ErrorAction Stop

if (-not $env:GOPROXY) {
    $env:GOPROXY = "https://goproxy.cn|https://proxy.golang.org|direct"
}

Push-Location $projectRoot
$previousCGO = $env:CGO_ENABLED
$previousCC = $env:CC
$previousCXX = $env:CXX
try {
    $env:CGO_ENABLED = "1"
    $env:CC = $clangCommand.Source
    $env:CXX = $clangxxCommand.Source
    Write-Host "Go race detection enabled with $($clangCommand.Source)"
    & $goCommand.Source test -race ./...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    $frontendPackage = Join-Path $projectRoot "frontend\package.json"
    if (Test-Path -LiteralPath $frontendPackage) {
        & $npmCommand.Source --prefix frontend test -- --run
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        & $npmCommand.Source --prefix frontend run build
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }
}
finally {
    $env:CGO_ENABLED = $previousCGO
    $env:CC = $previousCC
    $env:CXX = $previousCXX
    Pop-Location
}
