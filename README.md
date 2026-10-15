# vetter-sync

Agente read-only Vetter 5 → Cloudflare Worker + D1/R2.

- Vetter sigue siendo el sistema de carga. Esto solo lee una copia.
- `SPEC.md` es el spec completo.
- `agent/` será el binario Go (tarea programada Windows).
- `worker/` documenta el endpoint `POST /sync/vetter` (vive en `veterinaria-gachen`).

Estado: spec inicial, sin código.
