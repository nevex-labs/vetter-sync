# vetter-sync

Agente read-only Vetter 5 → Cloudflare Worker + D1/R2.

- Vetter sigue siendo el sistema de carga. Esto solo lee una copia.
- `SPEC.md` es el spec completo y sus fases.
- `agent/` es el binario Go. Hoy `inspect` lee la estructura de los DBF sin escribir; `sync` y `pair` vienen después.
- El receptor (`POST /api/sync/vetter`, tabla `vetter_rows`) ya existe en
  `veterinaria-gachen` (ADR-0016, paso 1).

## Docs

- [CLAUDE.md](./CLAUDE.md) — para Claude Code en la PC de la clínica.
- [AGENTS.md](./AGENTS.md) — reglas duras para trabajar en este repo.
- [docs/contract.md](./docs/contract.md) — **generado** desde el Worker. Qué
  mandar y cómo. No editar a mano.
- [docs/vetter-schema.md](./docs/vetter-schema.md) — DBF: confirmado vs
  hipótesis.
- [docs/claude-code-pc.md](./docs/claude-code-pc.md) — la visita, paso a paso.
- [docs/pc-runbook.md](./docs/pc-runbook.md) — instalar y operar en la PC.
- [docs/informe-visita.md](./docs/informe-visita.md) — plantilla del informe.
- [docs/agent-implementation.md](./docs/agent-implementation.md) — parser, hash y sync.
- [docs/quality.md](./docs/quality.md) — gates y reglas anti-slop de Go.
- [docs/release.md](./docs/release.md) — tags, firma y auto-update.
- [docs/open-questions.md](./docs/open-questions.md) — la checklist de la
  visita.

Estado: receptor implementado; agente Go pendiente.
