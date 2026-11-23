# Actualiza el agente a la última versión publicada, sin tocar nada a mano.
#
# Correr en PowerShell como administrador:
#   powershell -ExecutionPolicy Bypass -File C:\ProgramData\vetter-agent\instalar-actualizacion.ps1
#
# Baja el manifiesto y los dos binarios del último release, verifica el sha256
# de cada uno, para la tarea, reemplaza, la vuelve a arrancar y muestra el
# resultado. No toca config.json, secret.bin ni el snapshot.
#
# El agente 0.1.1 no tiene auto-update: esta corrida es la única manual.
# Después, cada release se instala solo cada 24 h.

$ErrorActionPreference = 'Stop'

$base = 'https://github.com/nevex-labs/vetter-sync/releases/latest/download'
$dir = 'C:\ProgramData\vetter-agent'
$task = 'Vetter Sync'

if (-not (Test-Path $dir)) {
    throw "No está $dir. Esto es para una PC que ya tiene el agente instalado."
}

$manifest = Invoke-RestMethod "$base/manifest.json"
Write-Host "Version publicada: $($manifest.version)"

function Save-Checked([string] $url, [string] $sha, [string] $dest) {
    Invoke-WebRequest $url -OutFile $dest -UseBasicParsing
    $got = (Get-FileHash $dest -Algorithm SHA256).Hash.ToLower()

    if ($got -ne $sha.ToLower()) {
        Remove-Item $dest -Force
        throw "El sha256 de $dest no coincide. No se instaló nada."
    }

    Write-Host "  ok $([System.IO.Path]::GetFileName($dest)) $got"
}

$newConsole = Join-Path $dir 'vetter-agent.exe.new'
$newTask = Join-Path $dir 'vetter-agent-task.exe.new'

Save-Checked $manifest.url $manifest.sha256 $newConsole
Save-Checked $manifest.task_url $manifest.task_sha256 $newTask

$before = & (Join-Path $dir 'vetter-agent.exe') version 2>$null
Write-Host "Version instalada: $before"

Stop-ScheduledTask -TaskName $task -ErrorAction SilentlyContinue
Start-Sleep -Seconds 3

foreach ($pair in @(@('vetter-agent.exe', $newConsole), @('vetter-agent-task.exe', $newTask))) {
    $target = Join-Path $dir $pair[0]

    if (Test-Path $target) {
        Copy-Item $target "$target.bak" -Force
    }

    Move-Item $pair[1] $target -Force
}

Start-ScheduledTask -TaskName $task

# 267009 = sigue corriendo: se espera hasta 5 minutos en vez de asumir que falló.
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 10
    $info = Get-ScheduledTaskInfo -TaskName $task
    if ($info.LastTaskResult -ne 267009) { break }
}

Write-Host "Version nueva: $(& (Join-Path $dir 'vetter-agent.exe') version)"
Get-ScheduledTaskInfo -TaskName $task | Select-Object LastRunTime, LastTaskResult, NextRunTime

Write-Host 'Listo. Si LastTaskResult no es 0, mirar el log de hoy en C:\ProgramData\vetter-agent\logs'
