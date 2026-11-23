# PC de la clínica — instalar y operar el agente

> Estado: instalado y sincronizando desde el 2026-11-18. La versión que corre
> se ve con `vetter-agent.exe version`; si está atrasada, se actualiza desde la
> ventana o con `instalar-actualizacion.ps1`. El auto-update baja los releases
> firmados solo, cada 24 h.

## Antes de tocar la PC

1. **Ok escrito del veterinario** (Ley 25.326). Está desde el 2026-11-18.
2. Dispositivo creado en el panel (Equipo → Dispositivos). El panel muestra el
   secreto una sola vez.
3. Cuenta de servicio decidida (ver abajo).

## Layout

```
C:\ProgramData\vetter-agent\
  vetter-agent.exe
  vetter-agent-task.exe   (sin consola, el que corre la tarea)
  config.json
  secret.bin              (DPAPI, nunca sale de la PC)
  snapshot.json
  copy\
  logs\
  update.checked / update.attempts   (auto-update)
```

`C:\ProgramData` no está sincronizado con OneDrive. No usar el Escritorio ni la
carpeta del usuario.

## `config.json`

```json
{
  "z_path": "\\\\Srvarchvete01\\c\\vetter5\\Datos",
  "copy_dir": "C:\\ProgramData\\vetter-agent\\copy",
  "snapshot": "C:\\ProgramData\\vetter-agent\\snapshot.json",
  "endpoint": "https://veterinariagachens.nevexlabs.workers.dev/api/sync/vetter",
  "install_id": "pc-vete",
  "mirror_path": "C:\\Users\\usuario\\OneDrive\\BKP Vetter5\\Datos",
  "max_rows_per_run": 5000,
  "keys": {
    "CLIENTES": "CODIGO",
    "PACIENTES": "CODIGOPACI",
    "VACUNAS": "ID_VACUNA",
    "VISITAS": "CODIGO+FECHA",
    "ADJUNTOS": "ID_ADJ"
  }
}
```

`mirror_path` es opcional: si está, cada corrida compara cuántos registros tiene
cada tabla entre el UNC y OneDrive y lo manda en `agent_error` cuando divergen.
Abre solo headers: no importa datos de la réplica.

El secreto no va en el JSON: se guarda con `vetter-agent pair` en DPAPI, con ACL
para la cuenta de la tarea.

## Cuenta y permisos

Hoy corre con la cuenta `usuario`, la misma del `pair`, con la contraseña
guardada en la tarea: arranca al iniciar Windows y no necesita que nadie abra
sesión. Para eso está `agent/instalar-tarea.ps1` (ver abajo).

- **Cuenta local dedicada** (opcional, más prolija): sin OneDrive y sin sesión.
  Hay que correr `pair` con esa cuenta o darle lectura a `secret.bin`, y darle
  lectura en el share.
- **SYSTEM sirve para el UNC** solo si la cuenta de equipo tiene permiso en el
  share; no puede autenticarse con credenciales de usuario.
- La cuenta necesita lectura en el share y escritura en `C:\ProgramData\vetter-agent`.

```powershell
icacls "C:\ProgramData\vetter-agent" /grant:r "VETE-PC\vetter-agent:(OI)(CI)M"
```

**El almacén de credenciales es por usuario.** Si la tarea corre con "no guardar
contraseña" (S4U) o con otra cuenta, no puede descifrar lo que guardó `pair`.

## Tarea programada

La de la primera visita corre solo con la sesión de `usuario` iniciada. Para que
corra sin sesión, con la misma cuenta:

```powershell
powershell -ExecutionPolicy Bypass -File C:\ProgramData\vetter-agent\instalar-tarea.ps1
```

Pide la contraseña, registra "Vetter Sync" al iniciar y cada 15 minutos, una sola
instancia, reintentos 1/2/5 min, límite de 10 minutos, y la arranca una vez para
ver el resultado.

- **Unidades mapeadas**: `Z:` es por sesión y una tarea desatendida no la ve.
  Por eso `z_path` va por UNC.
- Una corrida perdida no necesita recuperación: la siguiente compara contra el
  snapshot y manda lo que cambió.
- `Get-ScheduledTaskInfo -TaskName "Vetter Sync"` muestra el último código de
  salida.

## Instalación con la ventana (sin PowerShell)

`vetter-setup.exe` es el mismo binario sin consola, con tres botones:

1. **Pegar** lee el secreto del portapapeles (y lo vacía), le saca espacios,
   saltos de línea, comillas, BOM y caracteres de ancho cero, y lo deja en el
   campo. Es lo que rompe un pegado de RDP o de Word.
2. **Guardar pareo** valida el formato, hace un POST vacío al Worker y, si
   responde 200, lo cifra con DPAPI. Un 401 no guarda nada.
3. **Actualizar agente** baja del release firmado los dos binarios
   (`vetter-agent.exe` y `vetter-agent-task.exe`), verifica el sha256 y los
   deja en su lugar. La corrida siguiente ya usa la versión nueva.
4. **Instalar tarea** escribe `instalar-tarea.ps1` (va dentro del `.exe`) y lo
   abre. La cuenta y la contraseña de Windows las pide el diálogo del sistema:
   no pasan por este binario ni por una línea de comandos.

Arriba muestra la versión, la ruta del config y si el secreto está guardado.

Descarga: `https://github.com/nevex-labs/vetter-sync/releases/latest/download/vetter-setup.exe`

## Auto-update

Cada 24 h, antes del sync, el agente consulta el release firmado. Si hay una
versión mayor, la baja, verifica la firma y el sha256, y deja el binario nuevo
para la corrida siguiente. **No hay que tocar la PC.** Si la versión nueva falla
tres corridas seguidas, vuelve sola a la anterior (`vetter-agent.exe.old`).

La tarea usa `vetter-agent-task.exe` (sin consola); el agente baja el binario que
corresponde al que está corriendo, así que la ventana no aparece.

**Una vez sola**, para pasar de 0.1.1 (que no tiene auto-update) a la última:

```powershell
powershell -ExecutionPolicy Bypass -File C:\ProgramData\vetter-agent\instalar-actualizacion.ps1
```

Baja los dos binarios, verifica el sha256, para la tarea, reemplaza y la arranca.
No toca `config.json`, `secret.bin` ni el snapshot: no hay que volver a parear.

## Verificar

```powershell
# El endpoint está vivo y habla el contrato esperado
Invoke-RestMethod -Uri "https://veterinariagachens.nevexlabs.workers.dev/api/sync/vetter"
# → @{ ok = True; contract = 3 }

# Última corrida
Get-ScheduledTaskInfo -TaskName "Vetter Sync"

# Log (sin PII: conteos y vetter_id)
Get-ChildItem "C:\ProgramData\vetter-agent\logs\agent-*.log" | Sort-Object LastWriteTime | Select-Object -Last 1 | Get-Content -Tail 60
```

## Inspector (primera visita)

Antes de instalar nada, el inspector responde el schema real sin escribir una
fila ni sacar PII:

```powershell
.\vetter-agent.exe inspect `
  --dir "\\Srvarchvete01\c\vetter5\Datos" `
  --out "C:\ProgramData\vetter-inspector.json" `
  --stat CLIENTES.TELEFONO `
  --stat PACIENTES.CODIGOPACI
```

Devuelve, por cada DBF: versión, codepage (byte del header), cantidad de
registros, cuántos están borrados, tamaño de registro, campos y presencia del
`.FPT`. Con `--stat` agrega largos y separadores de un campo, nunca valores.

Qué contesta: encoding, borrados lógicos, volumen real, formato de teléfonos y
si `CODIGOPACI` es `cliente/paciente`. El JSON se lleva y se commitea como
evidencia en `docs/vetter-schema.md`.

No copia los DBF: lee el original en modo compartido. Si un archivo está
bloqueado lo informa y sigue con el resto.

## Diagnóstico

| Síntoma | Causa probable | Qué hacer |
|---|---|---|
| 401 | Secreto inválido o revocado | Revisar el pareo; si se revocó, crear un dispositivo nuevo |
| 400 | Body mal armado, falta `install_id`, más de 200 filas | Es un bug del agente, no reintentar |
| 413 | Lote de más de 1 MB | Bajar el tamaño de lote |
| 500 | Falló D1 | Reintentar con backoff |
| La tarea no corre | Cuenta sin permiso al UNC, o contraseña cambiada | Probar el UNC con `Test-Path` como esa cuenta |
| No copia | Vetter tiene el archivo bloqueado | Reintentar; nunca abrir el original para escritura |
| `changed: 0` todos los días | Normal si nadie cargó nada | Es el objetivo: mismo hash, ninguna escritura |

## Rollback

```powershell
schtasks /End /TN "Vetter Sync"
schtasks /Delete /TN "Vetter Sync" /F
```

Del lado del Worker no hay nada que deshacer: `vetter_rows` conserva lo
recibido y el agente no escribe en Vetter.

## Nunca

- Abrir los DBF con una herramienta que escriba, ni hacer `PACK`.
- Poner el `.exe`, el secreto o el `snapshot.json` en OneDrive o en el Escritorio.
- Correr dos instancias a la vez sobre el mismo `snapshot.json`.
- Leer la réplica de `OneDrive\BKP Vetter5`.
