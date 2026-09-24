param(
    [Parameter(Mandatory = $true)]
    [string]$Executable
)

$ErrorActionPreference = "Stop"
$source = (Resolve-Path -LiteralPath $Executable -ErrorAction Stop).Path
$temporaryRoot = [System.IO.Path]::GetFullPath((Join-Path ([System.IO.Path]::GetTempPath()) ("SmartEduDownloader-Smoke-" + [guid]::NewGuid().ToString("N"))))
$systemTemp = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
if (-not $temporaryRoot.StartsWith($systemTemp, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing to use a smoke directory outside the system temporary directory."
}

$first = $null
try {
    New-Item -ItemType Directory -Path $temporaryRoot | Out-Null
    $target = Join-Path $temporaryRoot "SmartEduDownloader.exe"
    Copy-Item -LiteralPath $source -Destination $target

    $first = Start-Process -FilePath $target -ArgumentList "--smoke-test" -WorkingDirectory $temporaryRoot -WindowStyle Hidden -PassThru
    $dataRoot = Join-Path $temporaryRoot ".smartedu-data"
    $ready = Join-Path $dataRoot "smoke.ready"
    $readyDeadline = [DateTime]::UtcNow.AddSeconds(15)
    while (-not (Test-Path -LiteralPath $ready)) {
        if ($first.HasExited) { throw "The smoke-test process exited before becoming ready (exit code $($first.ExitCode))." }
        if ([DateTime]::UtcNow -gt $readyDeadline) { throw "Timed out waiting for the smoke-test readiness record." }
        Start-Sleep -Milliseconds 100
        $first.Refresh()
    }

    $second = Start-Process -FilePath $target -ArgumentList "--smoke-test" -WorkingDirectory $temporaryRoot -WindowStyle Hidden -PassThru
    if (-not $second.WaitForExit(5000)) {
        Stop-Process -Id $second.Id -Force -ErrorAction SilentlyContinue
        throw "A second application instance did not exit within five seconds."
    }
    if ($second.ExitCode -ne 0) { throw "The second application instance exited with code $($second.ExitCode)." }

    Set-Content -LiteralPath (Join-Path $dataRoot "smoke.shutdown") -Value "shutdown" -Encoding Ascii
    if (-not $first.WaitForExit(10000)) {
        throw "The first application instance did not shut down within ten seconds."
    }
    if ($first.ExitCode -ne 0) { throw "The first application instance exited with code $($first.ExitCode)." }
    if (Get-Process -Id $first.Id -ErrorAction SilentlyContinue) { throw "The application process is still running after shutdown." }

    foreach ($item in Get-ChildItem -LiteralPath $temporaryRoot -Recurse -Force) {
        $resolved = [System.IO.Path]::GetFullPath($item.FullName)
        if (-not $resolved.StartsWith($temporaryRoot + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
            throw "Smoke-test output escaped the temporary directory: $resolved"
        }
    }
    Write-Host "Smoke test passed: single instance, graceful shutdown, and controlled storage verified."
}
finally {
    if ($first -and -not $first.HasExited) {
        Stop-Process -Id $first.Id -Force -ErrorAction SilentlyContinue
        $first.WaitForExit(3000) | Out-Null
    }
    if ((Test-Path -LiteralPath $temporaryRoot) -and $temporaryRoot.StartsWith($systemTemp, [System.StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}
