# CLAUDE.md — PC de la clínica

Este repo se clona en la PC de la veterinaria. El agente **ya está instalado y
sincronizando**; esta máquina se opera, no se desarrolla.

Acá no hay `wrangler` ni acceso al repo del Worker: el contrato está en
`docs/contract.md` y las reglas duras en `AGENTS.md`.

## Reglas que no se negocian

- **Nunca escribir en los DBF/FPT.** Ni `PACK`, ni reparar, ni renombrar. Solo
  lectura, siempre sobre el original en modo compartido.
- **Usar el UNC** (`\\Srvarchvete01\c\vetter5\Datos`), no la unidad `Z:`.
- **No sacar PII.** Nombres, teléfonos y notas no salen de la clínica. En los
  reportes van versiones, conteos y códigos internos, nada más.
- **No copiar los DBF** a OneDrive, al repo ni al Escritorio.
- **No tocar la configuración de Vetter ni de OneDrive.** Es decisión de la
  clínica.
- **No parear de nuevo** salvo que se pida: el dispositivo ya está pareado y el
  secreto, cifrado en `C:\ProgramData\vetter-agent\secret.bin`.
- **No implementar features acá.** El código vive en el repo; en la PC solo se
  instala y se verifica.

## Cómo se opera

Lo normal es la ventana `vetter-setup.exe`: pegar el secreto (solo si hay que
parear), actualizar el agente y registrar la tarea, con el diálogo de Windows
para la contraseña.

El paso a paso está en:

- `docs/claude-code-pc.md` — operación en la PC: actualizar, verificar, reportar.
- `docs/pc-runbook.md` — instalación, layout, diagnóstico y rollback.

## Estado

- Agente en `C:\ProgramData\vetter-agent\`, tarea **"Vetter Sync"** cada 15
  minutos, secreto con DPAPI.
- La versión que corre se ve con
  `& "C:\ProgramData\vetter-agent\vetter-agent.exe" version`. Si está atrasada,
  se actualiza desde la ventana o con `instalar-actualizacion.ps1`.
- El auto-update baja los releases firmados solo, cada 24 h. No hay que tocar
  la PC para eso.

## Si algo falla

1. `Get-ScheduledTaskInfo -TaskName "Vetter Sync"` → `LastTaskResult`.
2. `Get-Content "C:\ProgramData\vetter-agent\logs\agent.log" -Tail 60`.
3. El panel muestra `last_seen_at` y `last_error` de la PC.
4. `docs/pc-runbook.md` tiene la tabla de diagnóstico.

No reinicies Vetter, no muevas carpetas y no borres `secret.bin`,
`config.json` ni el snapshot para "probar".
