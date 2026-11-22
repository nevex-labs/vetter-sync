# vetter-sync

Agente read-only Vetter 5 → Cloudflare Worker + D1/R2.

- Vetter sigue siendo el sistema de carga. Esto solo lee una copia.
- `agent/` es el binario Go: copia los DBF de la lista blanca, hashea cada fila
  y manda lo que cambió. `inspect` lee estructura sin escribir; `pair` guarda el
  secreto; `sync` corre desde la tarea programada; `setup` es la ventana de
  instalación.
- El receptor (`POST /api/sync/vetter`, tabla `vetter_rows`) vive en
  `veterinaria-gachen` (ADR 0016). La proyección al dominio también.

Estado: en producción desde el 2026-11-22. Import completo (3.797 tutores,
5.385 mascotas, 3.656 dosis) y sync cada 15 minutos.

## Docs

- [CLAUDE.md](./CLAUDE.md) — para Claude Code en la PC de la clínica.
- [AGENTS.md](./AGENTS.md) — reglas duras para trabajar en este repo.
- [docs/contract.md](./docs/contract.md) — **generado** desde el Worker. Qué
  mandar y cómo. No editar a mano.
- [docs/claude-code-pc.md](./docs/claude-code-pc.md) — operar la PC: actualizar,
  verificar, reportar.
- [docs/pc-runbook.md](./docs/pc-runbook.md) — instalar, operar y diagnosticar.
- [docs/vetter-schema.md](./docs/vetter-schema.md) — DBF confirmado: campos,
  tipos y claves.
- [docs/agent-implementation.md](./docs/agent-implementation.md) — parser, hash
  y sync.
- [docs/informe-visita.md](./docs/informe-visita.md) — el informe del 2026-11-18.
- [docs/open-questions.md](./docs/open-questions.md) — lo que sigue abierto.
- [docs/quality.md](./docs/quality.md) — gates y reglas anti-slop de Go.
- [docs/release.md](./docs/release.md) — tags, firma y auto-update.
- [SPEC.md](./SPEC.md) — el diseño original.
