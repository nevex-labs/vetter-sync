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
Get-Content "C:\ProgramData\vetter-agent\logs\agent.log" -Tail 40
```

- `LastTaskResult = 0` es una corrida bien.
- En el log, cada corrida deja una línea `resumen` por tabla con `filas`,
  `pendientes`, `enviadas`, `ids_duplicados` y `error`.
- Si no hay cambios en Vetter, `enviadas` es 0: es el objetivo, no una falla.

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
Start-ScheduledTask -TaskName "Vetter Sync"
Start-Sleep -Seconds 90
Get-ScheduledTaskInfo -TaskName "Vetter Sync" | Select-Object LastTaskResult
Get-Content "C:\ProgramData\vetter-agent\logs\agent.log" -Tail 60
```

- `version` tiene que mostrar la última.
- `LastTaskResult = 0`.
- No tiene que aparecer `error` en las líneas `resumen`.

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
