# AGENTS.md — vetter-sync

Agente read-only que copia la base de Vetter 5 de la PC de la clínica y la
empuja al Worker `veterinariagachens`. Este repo se clona en esa PC, donde no
hay `wrangler` ni acceso al repo del Worker.

## Reglas duras

- **Nunca escribir en los DBF/FPT.** Ni abrirlos para escritura, ni `PACK`, ni
  reparar. Solo lectura, siempre sobre una copia local (`COPY_DIR`).
- **Copiar antes de parsear** con share de solo lectura. Nunca leer el original
  mientras Vetter lo está usando para escribir.
- **Nunca leer la réplica de OneDrive** (`C:\Users\...\OneDrive\BKP Vetter5`):
  diverge de producción. La fuente es `Z:` o su UNC.
- **Las unidades mapeadas son por sesión.** Una tarea programada que corre como
  SYSTEM o "sin que el usuario inicie sesión" no ve `Z:`. Usar el UNC
  (`\\Srvarchvete01\c\vetter5\Datos`).
- **El token no entra al repo ni a una carpeta sincronizada con OneDrive.** Va
  en un archivo aparte con ACL solo para la cuenta de la tarea.
- **Logs sin PII.** Conteos y `vetter_id`; nunca nombres, teléfonos ni notas.

## Qué hace y qué no

Hace: leer tablas de la lista blanca, armar el hash de cada fila, mandar lotes.

No hace: mapear a `tutors`, `pets`, `visits` ni `preventives`. Ese mapeo vive en
el Worker (ADR 0016). Si el agente empieza a conocer el dominio, se está
construyendo dos veces y una va a quedar vieja.

## Mapa

```
agent/                  binario Go (tarea programada de Windows)
docs/contract.md        GENERADO desde veterinaria-gachen. No editar.
docs/vetter-schema.md   DBF: qué está confirmado y qué sigue siendo hipótesis
docs/pc-runbook.md      instalar, operar y diagnosticar en la PC
docs/open-questions.md  lo que falta cerrar en la visita
SPEC.md                 el diseño y sus fases
```

## Contrato

`docs/contract.md` sale de `pnpm vetter:contract` en `veterinaria-gachen`. Si
cambia un límite, la lista blanca o la respuesta, se regenera y se commitea
acá. La cabecera trae el commit de origen.

Antes de mandar, el agente puede verificar contra qué habla:

```
GET /api/sync/vetter  →  { "ok": true, "contract": 1 }
```

Si `contract` no es el que el binario espera, no mandar: es más barato fallar
que escribir filas en un contrato que no se entiende.

## Build

```
GOOS=windows GOARCH=amd64 go build -o vetter-agent.exe ./agent
```

Un solo `.exe`, sin runtime.

## Evidencia

Cada afirmación sobre Vetter se prueba contra la copia o queda como pregunta en
`docs/open-questions.md`. El schema del 15/10 es hipótesis hasta la primera
visita.
