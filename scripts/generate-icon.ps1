$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

$projectRoot = Split-Path -Parent $PSScriptRoot
$output = Join-Path $projectRoot "build\appicon.png"
$windowsIcon = Join-Path $projectRoot "build\windows\icon.ico"
$webIcon = Join-Path $projectRoot "frontend\public\appicon.png"
$bitmap = [System.Drawing.Bitmap]::new(1024, 1024)
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$graphics.Clear([System.Drawing.Color]::Transparent)

$blue = [System.Drawing.SolidBrush]::new([System.Drawing.Color]::FromArgb(22, 104, 216))
$white = [System.Drawing.Pen]::new([System.Drawing.Color]::White, 48)
$white.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
$white.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
$white.LineJoin = [System.Drawing.Drawing2D.LineJoin]::Round
$shape = [System.Drawing.Drawing2D.GraphicsPath]::new()

try {
    $shape.AddArc(48, 48, 176, 176, 180, 90)
    $shape.AddArc(800, 48, 176, 176, 270, 90)
    $shape.AddArc(800, 800, 176, 176, 0, 90)
    $shape.AddArc(48, 800, 176, 176, 90, 90)
    $shape.CloseFigure()
    $graphics.FillPath($blue, $shape)

    $graphics.DrawLine($white, 512, 340, 512, 794)
    $graphics.DrawBeziers($white, [System.Drawing.Point[]]@(
        [System.Drawing.Point]::new(512, 408), [System.Drawing.Point]::new(445, 348),
        [System.Drawing.Point]::new(365, 326), [System.Drawing.Point]::new(228, 326)))
    $graphics.DrawLines($white, [System.Drawing.Point[]]@(
        [System.Drawing.Point]::new(228, 326), [System.Drawing.Point]::new(228, 710),
        [System.Drawing.Point]::new(352, 710)))
    $graphics.DrawBeziers($white, [System.Drawing.Point[]]@(
        [System.Drawing.Point]::new(352, 710), [System.Drawing.Point]::new(435, 710),
        [System.Drawing.Point]::new(480, 744), [System.Drawing.Point]::new(512, 794)))
    $graphics.DrawBeziers($white, [System.Drawing.Point[]]@(
        [System.Drawing.Point]::new(512, 408), [System.Drawing.Point]::new(555, 350),
        [System.Drawing.Point]::new(624, 326), [System.Drawing.Point]::new(680, 326)))
    $graphics.DrawLines($white, [System.Drawing.Point[]]@(
        [System.Drawing.Point]::new(512, 794), [System.Drawing.Point]::new(595, 710),
        [System.Drawing.Point]::new(796, 710), [System.Drawing.Point]::new(796, 594)))
    $graphics.DrawLines($white, [System.Drawing.Point[]]@(
        [System.Drawing.Point]::new(665, 468), [System.Drawing.Point]::new(726, 529),
        [System.Drawing.Point]::new(851, 398)))

    $bitmap.Save($output, [System.Drawing.Imaging.ImageFormat]::Png)
    Copy-Item -LiteralPath $output -Destination $webIcon -Force
    Write-Host "Generated $output"

    $sizes = @(16, 24, 32, 48, 64, 128, 256)
    $images = @()
    foreach ($size in $sizes) {
        $resized = [System.Drawing.Bitmap]::new($size, $size)
        $resizedGraphics = [System.Drawing.Graphics]::FromImage($resized)
        try {
            $resizedGraphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            $resizedGraphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
            $resizedGraphics.DrawImage($bitmap, 0, 0, $size, $size)
            $imageStream = [System.IO.MemoryStream]::new()
            try {
                $resized.Save($imageStream, [System.Drawing.Imaging.ImageFormat]::Png)
                $images += ,$imageStream.ToArray()
            }
            finally { $imageStream.Dispose() }
        }
        finally {
            $resizedGraphics.Dispose()
            $resized.Dispose()
        }
    }

    $iconStream = [System.IO.File]::Create($windowsIcon)
    $writer = [System.IO.BinaryWriter]::new($iconStream)
    try {
        $writer.Write([uint16]0)
        $writer.Write([uint16]1)
        $writer.Write([uint16]$sizes.Count)
        $offset = 6 + 16 * $sizes.Count
        for ($index = 0; $index -lt $sizes.Count; $index++) {
            $writer.Write([byte]($sizes[$index] % 256))
            $writer.Write([byte]($sizes[$index] % 256))
            $writer.Write([byte]0)
            $writer.Write([byte]0)
            $writer.Write([uint16]1)
            $writer.Write([uint16]32)
            $writer.Write([uint32]$images[$index].Length)
            $writer.Write([uint32]$offset)
            $offset += $images[$index].Length
        }
        foreach ($image in $images) { $writer.Write([byte[]]$image) }
        Write-Host "Generated $windowsIcon"
    }
    finally { $writer.Dispose() }
}
finally {
    $shape.Dispose()
    $white.Dispose()
    $blue.Dispose()
    $graphics.Dispose()
    $bitmap.Dispose()
}
