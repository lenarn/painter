# Подготовка архива для заливки на Beget через панель.
# Запуск из корня проекта:
#   powershell -ExecutionPolicy Bypass -File scripts\publish-pack.ps1
# Результат: publish\site-<дата>-<время>.zip (содержимое dist в корне архива)
#            + publish\site-<дата>-<время>.txt (манифест с отчётом).

$ErrorActionPreference = 'Stop'
$root   = Split-Path -Parent $PSScriptRoot
$web    = Join-Path $root 'web'
$dist   = Join-Path $web 'dist'
$pubDir = Join-Path $root 'publish'

function Format-Size([long]$bytes) {
    if ($bytes -ge 1MB) { return '{0:N1} МБ' -f ($bytes / 1MB) }
    if ($bytes -ge 1KB) { return '{0:N1} КБ' -f ($bytes / 1KB) }
    return "$bytes Б"
}

function Add-ListLines {
    param($lines, [string]$title, $items)
    if (-not $items -or $items.Count -eq 0) { return }
    $lines.Add(('{0} ({1}):' -f $title, $items.Count))
    foreach ($x in ($items | Select-Object -First 15)) { $lines.Add("  $x") }
    if ($items.Count -gt 15) { $lines.Add(('  ... ещё {0}' -f ($items.Count - 15))) }
    $lines.Add('')
}

function Get-PublishReport {
    param([string]$dist, [string]$pubDir)
    $lines = New-Object System.Collections.Generic.List[string]

    $files = @(Get-ChildItem -LiteralPath $dist -Recurse -File)
    $total = ($files | Measure-Object -Sum Length).Sum
    if (-not $total) { $total = 0 }
    $lines.Add(('Файлов: {0}    Общий размер: {1}' -f $files.Count, (Format-Size $total)))

    $latest = $null
    if (Test-Path -LiteralPath $pubDir) {
        $latest = Get-ChildItem -LiteralPath $pubDir -Filter 'site-*.zip' -File | Sort-Object Name | Select-Object -Last 1
    }
    if (-not $latest) {
        $lines.Add('Предыдущих публикаций нет — сравнивать не с чем (первая заливка).')
        return $lines
    }
    $lines.Add(('Сравнение с: {0}' -f $latest.Name))

    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem

    # Хэши файлов из архива — потоково, без распаковки на диск.
    $oldMap = @{}
    $sha = [System.Security.Cryptography.SHA256]::Create()
    $fs  = [System.IO.File]::OpenRead($latest.FullName)
    $zipOld = $null
    try {
        $zipOld = New-Object System.IO.Compression.ZipArchive($fs, [System.IO.Compression.ZipArchiveMode]::Read)
        foreach ($e in $zipOld.Entries) {
            if ($e.FullName.EndsWith('/')) { continue }
            $name = ($e.FullName -replace '^\./', '').Replace('\', '/')
            $s = $e.Open()
            try { $oldMap[$name] = [BitConverter]::ToString($sha.ComputeHash($s)).Replace('-', '') }
            finally { $s.Dispose() }
        }
    }
    finally {
        if ($zipOld) { $zipOld.Dispose() }
        $fs.Dispose()
        $sha.Dispose()
    }

    # Хэши новой сборки; относительные пути с '/' — как имена в архиве.
    $newMap = @{}
    foreach ($f in $files) {
        $rel = $f.FullName.Substring($dist.Length + 1).Replace('\', '/')
        $newMap[$rel] = (Get-FileHash -LiteralPath $f.FullName -Algorithm SHA256).Hash
    }

    $added   = @($newMap.Keys | Where-Object { -not $oldMap.ContainsKey($_) } | Sort-Object)
    $removed = @($oldMap.Keys | Where-Object { -not $newMap.ContainsKey($_) } | Sort-Object)
    $changed = @()
    foreach ($k in $newMap.Keys) {
        if ($oldMap.ContainsKey($k) -and $oldMap[$k] -ne $newMap[$k]) { $changed += $k }
    }
    $changed = @($changed | Sort-Object)

    $lines.Add(('Новых: {0}    Изменённых: {1}    Удалённых: {2}' -f $added.Count, $changed.Count, $removed.Count))
    $lines.Add('')
    Add-ListLines $lines 'Новые файлы' $added
    Add-ListLines $lines 'Изменённые файлы' $changed
    Add-ListLines $lines 'Файлы, которых нет в новой сборке' $removed
    $lines.Add('Удалённые файлы остаются на Beget до ручной очистки public_html.')
    return $lines
}

Write-Host ''
Write-Host '=== 1/3. Сборка сайта ===' -ForegroundColor Cyan
Push-Location $web
try {
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "Сборка завершилась с ошибкой (код $LASTEXITCODE)." }
}
finally { Pop-Location }

if (-not (Test-Path -LiteralPath $dist)) { throw 'web\dist не найден — заливать нечего.' }
$distFiles = @(Get-ChildItem -LiteralPath $dist -Recurse -File)
if ($distFiles.Count -eq 0) { throw 'web\dist пуст — заливать нечего.' }

Write-Host ''
Write-Host '=== 2/3. Отчёт о публикации ===' -ForegroundColor Cyan
$report = Get-PublishReport $dist $pubDir
$report | ForEach-Object { Write-Host $_ }

Write-Host ''
Write-Host '=== 3/3. Архив ===' -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $pubDir | Out-Null
$stamp = Get-Date -Format 'yyyy-MM-dd_HH-mm-ss'
$zip      = Join-Path $pubDir "site-$stamp.zip"
$manifest = Join-Path $pubDir "site-$stamp.txt"

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$out = [System.IO.File]::Create($zip)
$archive = $null
try {
    $archive = New-Object System.IO.Compression.ZipArchive($out, [System.IO.Compression.ZipArchiveMode]::Create)
    foreach ($f in $distFiles) {
        # имена записей — с '/': обратные слеши ломают распаковку на Linux
        $rel = $f.FullName.Substring($dist.Length + 1).Replace('\', '/')
        $entry = $archive.CreateEntry($rel, [System.IO.Compression.CompressionLevel]::Optimal)
        $es = $entry.Open()
        $in = [System.IO.File]::OpenRead($f.FullName)
        try { $in.CopyTo($es) }
        finally { $in.Dispose(); $es.Dispose() }
    }
}
finally {
    if ($archive) { $archive.Dispose() }
    $out.Dispose()
}
if (-not (Test-Path -LiteralPath $zip)) { throw 'Архив не создался.' }
$zipSize = (Get-Item -LiteralPath $zip).Length

$manifestLines = New-Object System.Collections.Generic.List[string]
$manifestLines.Add("Отчёт публикации — $stamp")
$manifestLines.Add('')
foreach ($l in $report) { $manifestLines.Add($l) }
$manifestLines.Add("Размер архива: $(Format-Size $zipSize)")
$manifestLines.Add('')
$manifestLines.Add('Заливка: панель Beget → файл-менеджер → public_html →')
$manifestLines.Add('загрузить site-*.zip → распаковать.')
$manifestLines.Add('Старые файлы поверх распаковки не удаляются — при больших')
$manifestLines.Add('списках удалённых почистить public_html перед распаковкой.')
$manifestLines | Out-File -FilePath $manifest -Encoding utf8

Write-Host ''
Write-Host "Архив:    $zip ($(Format-Size $zipSize))"
Write-Host "Манифест: $manifest"
Write-Host ''
Write-Host 'Готово. Панель Beget → файл-менеджер → public_html →'
Write-Host 'загрузить архив → распаковать.'