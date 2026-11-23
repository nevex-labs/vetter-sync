# Operar la PC de la clínica

El agente está instalado y sincronizando. Esta página es para cuando hay que
actualizarlo, dejarlo corriendo sin sesión o revisar por qué no manda.

Todo se hace en la PC, con el repo clonado o sin él. La fuente es siempre el
UNC `\\Srvarchvete01\c\vetter5\Datos`; nunca la réplica de OneDrive.

## 1. Ver el estado

```powershell
& "C:\ProgramData\vetter-agent\vetter-agent.exe" version
Get-ScheduledTask -TaskName "Vetter Sync" | Select-Object TaskName, State
Get-ScheduledTaskInfo -TaskName "Vetter Sync" |
  Select-Object LastRunTime, LastTaskResult, NextRunTime
Get-ChildItem "C:\ProgramData\vetter-agent\logs\agent-*.log" | Sort-Object LastWriteTime | Select-Object -Last 1 | Get-Content -Tail 60
```

- `LastTaskResult = 0` es una corrida bien. `267009` es "sigue corriendo":
  esperar, no es una falla. `267011` es "nunca corrió". `1` es error del
  agente: mirar el log. `0x8007052E` es cuenta o contraseña incorrecta.
- En el log, cada corrida deja una línea `resumen` por tabla con `filas`,
  `pendientes`, `confirmadas`, `sin_cambios` y `error`. Los `ids_duplicados`
  salen en la línea `tabla leída`, no en `resumen`.
- La línea `heartbeat` con `received` y `agent_error` es la prueba de que el
  Worker respondió. Es lo más cercano al panel que hay en la PC.
- Si no hay cambios en Vetter, `pendientes` es 0: es el objetivo, no una falla.

## 2. Actualizar

La forma normal es la ventana, sin consola:

```powershell
$dir = "$env:TEMP\vetter-setup"
New-Item -ItemType Directory -Force $dir | Out-Null
Invoke-WebRequest "https://github.com/nevex-labs/vetter-sync/releases/latest/download/vetter-setup.exe" -OutFile "$dir\vetter-setup.exe"
Start-Process "$dir\vetter-setup.exe" -Verb RunAs
```

En la ventana:

1. **Pegar** (solo si hay que parear): lee el portapapeles, lo vacía y limpia
   espacios, saltos, comillas y BOM.
2. **Guardar pareo**: valida contra el servidor y cifra con DPAPI. Un 401 no
   guarda nada.
3. **Actualizar agente**: baja los dos binarios del release firmado, verifica
   sha256 y los deja en su lugar.
4. **Instalar tarea**: abre el instalador y pide cuenta y contraseña de Windows
   en el diálogo del sistema.

Si SmartScreen dice "Windows protegió tu PC" (el `.exe` no está firmado con un
certificado comercial): Más información → Ejecutar de todas formas.

Fallback por consola, mismo resultado:

```powershell
powershell -ExecutionPolicy Bypass -File "C:\ProgramData\vetter-agent\instalar-actualizacion.ps1"
```

## 3. Verificar después de actualizar

```powershell
# Qué binario corre la tarea y con qué cuenta
(Get-ScheduledTask -TaskName "Vetter Sync").Principal | Select-Object UserId, LogonType, RunLevel
Get-FileHash "C:\ProgramData\vetter-agent\vetter-agent-task.exe" -Algorithm SHA256
& "C:\ProgramData\vetter-agent\vetter-agent.exe" version

# Una corrida y su resultado
Start-ScheduledTask -TaskName "Vetter Sync"
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 10
    $info = Get-ScheduledTaskInfo -TaskName "Vetter Sync"
    if ($info.LastTaskResult -ne 267009) { break }
}
$info | Select-Object LastRunTime, LastTaskResult
Get-ChildItem "C:\ProgramData\vetter-agent\logs\agent-*.log" | Sort-Object LastWriteTime | Select-Object -Last 1 | Get-Content -Tail 60
```

- La tarea ejecuta `vetter-agent-task.exe`, no `vetter-agent.exe`: el `version`
  prueba el binario de consola, el hash del task exe prueba el que corre.
- El hash tiene que coincidir con `task_sha256` del manifiesto del release.
- `LogonType` pasa de `Interactive` a `Password` cuando la tarea corre sin
  sesión.
- `LastTaskResult = 0`; `267009` es "sigue corriendo", no una falla.
- `LastRunTime` tiene que ser posterior a la actualización: si no, se está
  leyendo la corrida de la versión vieja.
- No tiene que aparecer `error` en las líneas `resumen`, ni `contrato distinto`
  ni `401`.

## 4. Aviso de copias divergentes (opcional)

Si existe `C:\Users\usuario\OneDrive\BKP Vetter5\Datos` y se agrega al
`config.json`:

```json
"mirror_path": "C:\\Users\\usuario\\OneDrive\\BKP Vetter5\\Datos"
```

cada corrida compara la cantidad de registros de las 5 tablas entre el UNC y
OneDrive y lo reporta como `mirror: …` en `agent_error`. Lee solo headers: no
importa datos de la réplica. Que las cantidades coincidan no garantiza que el
contenido sea igual.

## 5. Reportar (sin PII)

1. Versión antes y después.
2. Texto literal de lo que dijo la ventana o el script.
3. `LastTaskResult`.
4. Las líneas `resumen` del log, tal cual.
5. Cualquier error, con el mensaje completo y sin nombres ni teléfonos.

## Lo que no se hace

- Abrir los DBF con cualquier cosa que escriba, ni `PACK`, ni reparar.
- Copiar DBF o `.FPT` a OneDrive, al repo o al Escritorio.
- Tocar la configuración de Vetter o de OneDrive.
- Borrar `secret.bin`, `config.json` o el snapshot "para probar".
- Parear de nuevo si el dispositivo ya está pareado: el secreto es uno solo y
  se revoca desde el panel.
