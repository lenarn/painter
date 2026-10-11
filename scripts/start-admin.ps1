# start-admin.ps1 — запуск локальной админки сайта (localhost:8150).
# Собирает admin.exe из исходников (Go), стартует админку и открывает браузер.
# Остановка админки — Ctrl+C в её окне или закрыть окно.
# Превью сайта — отдельно: кнопка «Остановить превью» на странице «Сборка».
param([int]$Port = 8150)

$here  = Split-Path -Parent $MyInvocation.MyCommand.Path
$root  = Split-Path -Parent $here
$admin = Join-Path $root 'tools\admin'
$exe   = Join-Path $admin 'admin.exe'

# уже работает? второй экземпляр не нужен
if (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue) {
    Write-Host "Порт $Port уже слушает — админка, похоже, уже работает:"
    Write-Host "    http://localhost:$Port"
    Read-Host 'Enter — выход'
    exit
}

Set-Location $admin
Write-Host 'Сборка админки...'
go build -o admin.exe .
if ($LASTEXITCODE -ne 0) {
    Write-Host ''
    Write-Host 'Сборка не удалась. Если админка уже запущена (её окно открыто),'
    Write-Host 'сначала закрой его. Иначе — покажи это сообщение агенту.'
    Read-Host 'Enter — выход'
    exit 1
}

Write-Host "Запуск: http://localhost:$Port"
Start-Process -FilePath $exe -WorkingDirectory $root -ArgumentList '-port', $Port
Start-Sleep -Milliseconds 800
Start-Process "http://localhost:$Port"