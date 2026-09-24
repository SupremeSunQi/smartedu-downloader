$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go.exe -CommandType Application -ErrorAction Stop
$npmCommand = Get-Command npm.cmd -CommandType Application -ErrorAction Stop
$wailsCommand = Get-Command wails.exe -CommandType Application -ErrorAction Stop
if (-not $env:GOPROXY) {
    $env:GOPROXY = "https://goproxy.cn|https://proxy.golang.org|direct"
}

Push-Location $projectRoot
try {
    & $npmCommand.Source --prefix frontend ci --prefer-offline --no-audit
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    & $npmCommand.Source --prefix frontend test -- --run
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    & $npmCommand.Source --prefix frontend run build
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & $goCommand.Source test ./...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    & $goCommand.Source vet ./...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & (Join-Path $PSScriptRoot "generate-icon.ps1")
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & $wailsCommand.Source build -platform windows/amd64 -clean -o SmartEduDownloader.exe
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    $executable = Join-Path $projectRoot "build\bin\SmartEduDownloader.exe"
    if (-not (Test-Path -LiteralPath $executable)) {
        throw "Wails completed without producing $executable"
    }
    $hash = (Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash
    Write-Host "Built: $executable"
    Write-Host "SHA256: $hash"
}
finally {
    Pop-Location
}
