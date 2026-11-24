# Prompt para la PC

Para pegar en Claude Code en la PC de la clínica. Es corto a propósito: el paso a
paso vive en `docs/claude-code-pc.md`, y acá van las cuatro cosas que el doc no
puede saber y las paradas.

Si el repo no está o el `git pull` falla, el prompt solo trae los comandos
mínimos; el resto de las paradas están en la tabla de abajo.

---

```text
Sos Claude Code en la PC de la veterinaria (Windows). Objetivo de esta visita:
dejar el agente de Vetter en la versión 0.3.2 y la tarea "Vetter Sync"
corriendo sin sesión iniciada.

ANTES DE EMPEZAR
- git pull en el clon del repo. Si el repo no está o el pull falla, seguí con
  este prompt igual: tiene los comandos y las paradas.
- Leé docs/claude-code-pc.md. Si el doc y este prompt no coinciden, gana este
  prompt. Si un comando del doc falla, PARÁ y reportá; no lo arregles.
- La versión esperada es 0.3.2, literal. Si la PC ya la tiene, salteá
  "Actualizar agente" y andá directo a verificar la tarea.

REGLAS
- No parear: el dispositivo ya está pareado y el secreto está guardado.
- No editar config.json y no activar "mirror_path" en esta visita.
- No tocar la configuración de Vetter ni de OneDrive.
- No abrir los DBF para escritura, ni PACK, ni reparar.
- No pedir ni mostrar la contraseña de Windows: la escribe la persona en el
  diálogo del sistema. Vos solo esperás.
- No tocar los `.old` del auto-update ni los `.bak` de los scripts.
- Si GitHub no responde y no se puede bajar el `.exe`, PARÁ: no hay camino sin
  conexión, y el `vetter-agent.exe` que está en el repo es de la visita, viejo.

PASOS

1. Estado de partida (no cambiar nada):
   & "C:\ProgramData\vetter-agent\vetter-agent.exe" version
   (Get-ScheduledTask -TaskName "Vetter Sync").Principal | Select-Object UserId, LogonType, RunLevel
   Get-ScheduledTaskInfo -TaskName "Vetter Sync" | Select-Object LastRunTime, LastTaskResult
   Get-ChildItem "C:\ProgramData\vetter-agent\logs\agent-*.log" | Sort-Object LastWriteTime | Select-Object -Last 1 | Get-Content -Tail 40

   Anotá: versión, UserId, LogonType, LastTaskResult. El UserId tiene que ser
   "usuario" (el COMPUTERNAME\usuario). Si es otra cuenta, PARÁ.

2. Actualizar:
   $dir = "$env:TEMP\vetter-setup"; New-Item -ItemType Directory -Force $dir | Out-Null
   Invoke-WebRequest "https://github.com/nevex-labs/vetter-sync/releases/latest/download/vetter-setup.exe" -OutFile "$dir\vetter-setup.exe"
   Start-Process "$dir\vetter-setup.exe" -Verb RunAs

   Si aparece SmartScreen: Más información → Ejecutar de todas formas.
   En la ventana: "Actualizar agente". Tiene que decir "Actualizado a 0.3.2" y
   "listo" para los dos binarios. Si dice "Ya está la última versión", seguí.

3. Tarea (solo si LogonType no es "Password"):
   En la misma ventana, "Instalar tarea". Se abre PowerShell y pide cuenta y
   contraseña: la persona escribe usuario y su contraseña. Si el diálogo ofrece
   otro usuario, la persona lo corrige antes de aceptar. Esperá a que la ventana
   muestre el resultado. Si `Register-ScheduledTask` pisa una tarea existente,
   es lo esperado: después se verifica con Principal.

4. Verificar:
   $m = Invoke-RestMethod "https://github.com/nevex-labs/vetter-sync/releases/latest/download/manifest.json"
   Get-FileHash "C:\ProgramData\vetter-agent\vetter-agent-task.exe" -Algorithm SHA256
   # el hash tiene que ser $m.task_sha256
   (Get-ScheduledTask -TaskName "Vetter Sync").Principal | Select-Object UserId, LogonType
   # LogonType = Password
   Start-ScheduledTask -TaskName "Vetter Sync"
   for ($i = 0; $i -lt 30; $i++) { Start-Sleep -Seconds 10; $info = Get-ScheduledTaskInfo -TaskName "Vetter Sync"; if ($info.LastTaskResult -ne 267009) { break } }
   $info | Select-Object LastRunTime, LastTaskResult
   Get-ChildItem "C:\ProgramData\vetter-agent\logs\agent-*.log" | Sort-Object LastWriteTime | Select-Object -Last 1 | Get-Content -Tail 60

PARADAS (no adivines; si cae acá, reportá y esperá)
- 267009: sigue corriendo. Esperá, no es falla.
- 267011: nunca corrió. Revisá que la tarea esté habilitada y reportá.
- 0x8007052E: cuenta o contraseña incorrecta. No reintentes.
- 1: error del agente. Leé el log y reportá la línea.
- Versión más nueva que 0.3.2: PARÁ y reportá; no downgradees.
- "contrato distinto" o 401 en el log: PARÁ; no toques el secreto.
- "el DBF cambió durante la copia": aviso, no parar; la corrida siguiente lo
  reintenta.
- "error" no vacío en una línea `resumen`: reportá esa tabla y no sigas.

REPORTE (sin PII; bloques con etiquetas, log literal al final)
pc:
version_antes: / version_despues: / version_esperada: 0.3.2
task_exe_sha256_ok: sí|no
tarea: UserId= LogonType= LastRunTime= LastTaskResult=
ventana_o_script: <texto literal>
heartbeat: received= agent_error=
paradas: <caso y por qué>

--- log (literal, desde la corrida posterior a la actualización) ---
<pegar>

NO me pidas: last_seen_at ni conteos del servidor; si el bot ve los datos; si
sincroniza después de un reinicio sin sesión; si el contenido de UNC y OneDrive
es igual; cuántas filas hay en el servidor.
```

---

## Por qué este formato

- **Puntero, no pegote**: el paso a paso vive en `docs/claude-code-pc.md`, que se
  actualiza con el repo. El prompt solo trae la versión literal, la regla de
  conflicto, las prohibiciones de esta visita y el reporte.
- **Pegote igual cuando**: no hay repo, el `git pull` falla, o la visita no es
  rutinaria (re-pareo, cambio de cuenta, rollback).
- **Paradas explícitas**: `267009` no es una falla y con una espera fija de 60
  segundos se ve seguido.
- **La tarea corre `vetter-agent-task.exe`**: el `version` del binario de consola
  no prueba qué corre la tarea; el hash sí.
- **`LogonType`** pasa de `Interactive` a `Password`: esa es la prueba de que no
  depende de la sesión, sin reiniciar la PC.
