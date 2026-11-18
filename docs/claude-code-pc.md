# Claude Code en la PC — primera y única visita

Objetivo: **sacar el schema real de Vetter y dejar la PC lista para operar
desde acá para siempre.** No se instala el agente todavía: `pair` y `sync` no
existen en este binario. Lo que sigue después se hace en esta misma máquina.

## Antes de tocar nada

1. `git log --oneline -1` y `.\vetter-agent.exe version` → `vetter-agent 0.1.1`.
   Si el exe no está, `git pull`; si hay que rebuildear:
   `cd agent; GOOS=windows GOARCH=amd64 go build -o ../vetter-agent.exe .`
2. **Conectividad**:
   ```powershell
   curl.exe -s https://veterinariagachens.nevexlabs.workers.dev/api/sync/vetter
   # {"ok":true,"contract":2}
   ```
   Timeout o error de red = firewall/proxy. Reportalo y **no sigas** con el
   sync: primero hay que resolver la salida.
3. **Acceso al share**:
   ```powershell
   Test-Path "\\Srvarchvete01\c\vetter5\Datos"
   Get-ChildItem "\\Srvarchvete01\c\vetter5\Datos" -Filter *.DBF |
     Select-Object Name, Length, LastWriteTime
   ```

## Inspect

```powershell
.\vetter-agent.exe inspect `
  --dir "\\Srvarchvete01\c\vetter5\Datos" `
  --out "$env:TEMP\vetter-inspector.json" `
  --stat CLIENTES.TELEFONO `
  --stat PACIENTES.CODIGOPACI
```

Lee el original en modo compartido, no copia nada y no escribe en el origen.
Si un archivo está bloqueado lo informa en `errors` y sigue con el resto.

## Cómo leer el resultado

| Campo del JSON | Qué contesta |
|---|---|
| `header.language_driver` + `header.encoding` | Codepage real (CP850/CP1252). Es lo que define la decodificación antes del hash. |
| `records.deleted` | Si Vetter marca borrados con `0x2A`. Si da 0 en una tabla donde se borraron filas, probablemente haga `PACK`: hay que decirlo, no adivinarlo. |
| `records.header` vs `records.computed_from_size` + `records.match` | Si el archivo es consistente o se copió a mitad de una escritura. |
| `stats[].lengths` / `separators` de `TELEFONO` | Formato real: cuántos números, con qué separadores, cuántos son solo dígitos. |
| `stats[].prefix_lengths` de `CODIGOPACI` | Si el prefijo antes de `/` es el código del cliente. |
| `memo` | Si hay `.FPT` (memos) y cuánto pesa. |
| `fields` | Nombres, tipos y largos reales, para escribir el parser. |

No interpretes valores de filas: el reporte no los trae.

## Reportar

1. Llená `docs/informe-visita.md` con los números y las respuestas de las
   preguntas abiertas (`docs/open-questions.md`).
2. Pegá el resumen de consola en el chat. El JSON no tiene PII: se puede
   adjuntar o commitear como evidencia.
3. Anotá cualquier `errors` con el nombre del archivo y el mensaje, sin
   intentar arreglarlo.

## Lo que no se hace en esta visita

- No `pair`, no `sync`, no tareas programadas.
- No abrir los DBF con utilidades que escriban.
- No copiar DBF ni `.FPT` a OneDrive, al repo o al Escritorio.
- No tocar la configuración de Vetter ni la réplica de OneDrive.

## Después, desde esta misma máquina

Cuando el agente completo esté publicado:

1. `git pull` en este repo.
2. En el panel: Equipo → Dispositivos → Conectar PC. Muestra el secreto una vez.
3. `.\vetter-agent.exe pair` — **el secreto se pega en el prompt, nunca en el
   chat de Claude Code**.
4. Instalar la tarea (`docs/pc-runbook.md`), correrla una vez y verificar el
   `last_seen_at` en el panel.

Si algo falla, el panel muestra `last_seen_at` y `last_error`; los logs locales
están en `C:\ProgramData\vetter-agent\logs`. No hace falta volver a la PC.
