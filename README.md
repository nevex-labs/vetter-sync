# vetter-sync

Agente read-only Vetter 5 → Cloudflare Worker + D1/R2.

- Vetter sigue siendo el sistema de carga. Esto solo lee una copia.
- `SPEC.md` es el spec completo y sus fases.
- `agent/` será el binario Go (tarea programada de Windows).
- El receptor (`POST /api/sync/vetter`, tabla `vetter_rows`) ya existe en
  `veterinaria-gachen` (ADR-0016, paso 1).

## Docs

- [AGENTS.md](./AGENTS.md) — reglas duras para trabajar en este repo.
- [docs/contract.md](./docs/contract.md) — **generado** desde el Worker. Qué
  mandar y cómo. No editar a mano.
- [docs/vetter-schema.md](./docs/vetter-schema.md) — DBF: confirmado vs
  hipótesis.
- [docs/pc-runbook.md](./docs/pc-runbook.md) — instalar y operar en la PC.
- [docs/open-questions.md](./docs/open-questions.md) — la checklist de la
  visita.

Estado: receptor implementado; agente Go pendiente.
