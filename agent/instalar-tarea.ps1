# Registra "Vetter Sync" para que corra sin que nadie inicie sesión.
#
# La tarea de la primera visita corre solo con la sesión de `usuario` abierta: si
# la PC reinicia de madrugada, no sincroniza hasta el login. Esta variante guarda
# la contraseña de la cuenta y arranca al iniciar Windows.
#
# Correr en PowerShell como administrador:
#   powershell -ExecutionPolicy Bypass -File C:\ProgramData\vetter-agent\instalar-tarea.ps1
#
# La cuenta tiene que ser la misma con la que se hizo `pair`, o una que tenga
# lectura sobre secret.bin (icacls ... /grant <cuenta>:R). Con "no guardar
# contraseña" (S4U) la tarea NO llega al UNC: por eso pide la contraseña.
#
# El binario tiene que estar compilado sin consola (-H=windowsgui): con consola,
# cada corrida abre y cierra una ventana negra.

$exe = 'C:\ProgramData\vetter-agent\vetter-agent-task.exe'

$action = New-ScheduledTaskAction -Execute $exe `
    -Argument 'sync --config "C:\ProgramData\vetter-agent\config.json"'

$trigger = New-ScheduledTaskTrigger -AtStartup
$trigger.Repetition = (New-ScheduledTaskTrigger -Once -At (Get-Date) `
    -RepetitionInterval (New-TimeSpan -Minutes 15)).Repetition

$settings = New-ScheduledTaskSettingsSet `
    -MultipleInstances IgnoreNew `
    -StartWhenAvailable `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit (New-TimeSpan -Minutes 10)

$cred = Get-Credential -UserName "$env:COMPUTERNAME\$env:USERNAME" -Message 'Cuenta con la que corre Vetter Sync'

Register-ScheduledTask -TaskName 'Vetter Sync' `
    -Action $action -Trigger $trigger -Settings $settings `
    -User $cred.UserName -Password $cred.GetNetworkCredential().Password -RunLevel Limited -Force

Start-ScheduledTask -TaskName 'Vetter Sync'

for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 10
    $info = Get-ScheduledTaskInfo -TaskName 'Vetter Sync'
    if ($info.LastTaskResult -ne 267009) { break }
}
Get-ScheduledTaskInfo -TaskName 'Vetter Sync' | Select-Object LastRunTime, LastTaskResult, NextRunTime
