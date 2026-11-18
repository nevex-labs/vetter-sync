# CLAUDE.md — PC de la clínica

Este repo se clona en la PC de la veterinaria. Acá no hay `wrangler` ni acceso
al repo del Worker: el contrato está en `docs/contract.md` y las reglas duras
en `AGENTS.md`.

**La PC se toca una sola vez.** Todo lo que siga se hace desde esta misma
máquina con Claude Code, sin volver.

## Reglas que no se negocian

- **Nunca escribir en los DBF/FPT.** Ni `PACK`, ni reparar, ni renombrar. Solo
  lectura, siempre sobre el original en modo compartido.
- **Usar el UNC** (`\\Srvarchvete01\c\vetter5\Datos`), no la unidad `Z:`.
- **No sacar PII.** El inspector no imprime valores de campos; no los pidas ni
  los pegues en el chat. Nombres, teléfonos y notas no salen de la clínica.
- **No correr `pair` ni `sync`**: este binario todavía no los tiene. Solo
  `inspect` y `version`.
- **No copiar los DBF** a OneDrive, al repo ni al Escritorio.

## Empezar

Leé `docs/claude-code-pc.md`, que tiene el paso a paso de la visita, cómo
interpretar el JSON y qué reportar. El binario ya viene en el clon:
`.\vetter-agent.exe version` tiene que decir `0.1.0`.

## Después del inspect

Implementá `pair` y `sync` en `agent/` siguiendo `docs/agent-implementation.md`.
Los vectores de hash están ahí: no los inventes. No mapees a `tutors`, `pets`,
`visits` ni `preventives`; eso vive en el Worker.

Si el `.exe` no está o el repo está viejo: `git pull`. Si hay que rebuildear,
`cd agent; GOOS=windows GOARCH=amd64 go build -o ../vetter-agent.exe .`.
