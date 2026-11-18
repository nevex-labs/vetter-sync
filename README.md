# vetter-sync

Agente read-only Vetter 5 → Cloudflare Worker + D1/R2.

- Vetter sigue siendo el sistema de carga. Esto solo lee una copia.
- `SPEC.md` es el spec completo.
- `agent/` será el binario Go (tarea programada Windows).
- El receptor (`POST /api/sync/vetter`, tabla `vetter_rows`) ya existe en `veterinaria-gachen` (ADR-0016, paso 1).

Estado: receptor implementado; agente Go pendiente.
