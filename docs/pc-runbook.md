# PC de la clínica — instalar y operar el agente

> Estado: el `.exe` todavía no existe (fase 3 del `SPEC.md`). Esto es el
> procedimiento objetivo; se ajusta cuando el binario exista y se pruebe en la
> PC.

## Antes de tocar la PC

1. **Ok escrito del veterinario** (Ley 25.326). Sin eso no se manda una fila.
2. Token generado. **No se carga en el Worker hasta tener el ok**: el endpoint
   falla cerrado con 401 si el secret no está.
3. Cuenta de servicio decidida (ver abajo).

## Layout

```
C:\ProgramData\vetter-agent\
  vetter-agent.exe
  config.json
  token.txt
  snapshot.db
  copy\
  logs\
```

`C:\ProgramData` no está sincronizado con OneDrive. No usar el Escritorio ni la
carpeta del usuario.

## `config.json`

```json
{
  "z_path": "\\\\Srvarchvete01\\c\\vetter5\\Datos",
  "copy_dir": "C:\\ProgramData\\vetter-agent\\copy",
  "snapshot_db": "C:\\ProgramData\\vetter-agent\\snapshot.db",
  "endpoint": "https://veterinariagachens.nevexlabs.workers.dev/api/sync/vetter",
  "install_id": "pc-vete"
}
```

El token va en `token.txt`, no en el JSON.

## Cuenta y permisos

- **Cuenta local dedicada**, sin OneDrive y sin sesión abierta. Si se usa la
  cuenta del vete, un cambio de contraseña deja la tarea sin correr.
- **SYSTEM sirve para el UNC** solo si la cuenta de equipo tiene permiso en el
  share; no puede autenticarse con credenciales de usuario.
- La cuenta necesita lectura en el share y escritura en `C:\ProgramData\vetter-agent`.

```powershell
icacls "C:\ProgramData\vetter-agent" /grant:r "VETE-PC\vetter-agent:(OI)(CI)M"
icacls "C:\ProgramData\vetter-agent\token.txt" /inheritance:r /grant:r "VETE-PC\vetter-agent:(R)"
```

Verificar con `icacls "C:\ProgramData\vetter-agent\token.txt"`: la cuenta de la
tarea tiene que ser la única con acceso.

## Tarea programada

```powershell
schtasks /Create /TN "Vetter Sync" /SC MINUTE /MO 5 /RL LIMITED `
  /RU "VETE-PC\vetter-agent" /RP * `
  /TR "\"C:\ProgramData\vetter-agent\vetter-agent.exe\" --config \"C:\ProgramData\vetter-agent\config.json\""
```

- `/RP *` pide la contraseña de la cuenta.
- `/RL LIMITED`: no hace falta administrador.
- **Unidades mapeadas**: `Z:` es por sesión y una tarea desatendida no la ve.
  Por eso `z_path` va por UNC.
- Una corrida perdida (PC apagada) no necesita recuperación: la siguiente
  compara contra `snapshot.db` y manda todo lo que cambió.

## Verificar

```powershell
# El endpoint está vivo y habla el contrato esperado
Invoke-RestMethod -Uri "https://veterinariagachens.nevexlabs.workers.dev/api/sync/vetter" `
  -Headers @{ Authorization = "Bearer <token>" }
# → @{ ok = True; contract = 1 }

# Última corrida
Get-ScheduledTaskInfo -TaskName "Vetter Sync"

# Log (sin PII: conteos y vetter_id)
Get-Content "C:\ProgramData\vetter-agent\logs\agent.log" -Tail 30
```

## Diagnóstico

| Síntoma | Causa probable | Qué hacer |
|---|---|---|
| 401 | Token mal, o `VETTER_SYNC_TOKEN` sin cargar en el Worker | Revisar `token.txt`; no reintentar en loop |
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
- Poner el `.exe`, el token o el `snapshot.db` en OneDrive o en el Escritorio.
- Correr dos instancias a la vez sobre el mismo `snapshot.db`.
- Leer la réplica de `OneDrive\BKP Vetter5`.
