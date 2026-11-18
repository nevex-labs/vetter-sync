# AGENTS.md — vetter-sync

Si sos Claude Code en la PC de la clínica, leé `CLAUDE.md` y
`docs/claude-code-pc.md`: tienen el paso a paso de la única visita.

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
- **El secreto de instalación no entra al repo ni a una carpeta sincronizada
  con OneDrive.** Se guarda una vez con `vetter-agent pair` y vive en el
  almacén de la cuenta que corre la tarea (Credential Manager o DPAPI). La
  cuenta del pareo y la de la tarea tienen que ser la misma.
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

## Pareo

`vetter-agent pair` se corre **una vez**, con la misma cuenta que va a usar la
tarea programada, y guarda el secreto. Después cada corrida se autentica sola:
no hay token que rotar a mano. Si se revoca desde el panel, la PC deja de
sincronizar y hay que crear un dispositivo nuevo. Ver `docs/pc-runbook.md`.

## Contrato

`docs/contract.md` sale de `pnpm vetter:contract` en `veterinaria-gachen`. Si
cambia un límite, la lista blanca o la respuesta, se regenera y se commitea
acá. La cabecera trae el commit de origen.

Antes de mandar, el agente puede verificar contra qué habla:

```
GET /api/sync/vetter  →  { "ok": true, "contract": 2 }
```

El GET es público: sirve para probar conectividad desde la PC sin credencial.
La credencial se prueba con un POST de lote vacío.

Si `contract` no es el que el binario espera, no mandar: es más barato fallar
que escribir filas en un contrato que no se entiende.

## Build

```
cd agent
GOOS=windows GOARCH=amd64 go build -o ../vetter-agent.exe .
```

Un solo `.exe`, sin runtime ni dependencias.

El inspector de la primera visita:

```
vetter-agent.exe inspect --dir "\\Srvarchvete01\c\vetter5\Datos" --out inspector.json \
  --stat CLIENTES.TELEFONO --stat PACIENTES.CODIGOPACI
```

Lee la estructura y cuenta sin escribir y sin sacar valores: nombres de campo,
tipos, codepage, conteos, borrados y agregados de largos/separadores.

## Evidencia

Cada afirmación sobre Vetter se prueba contra la copia o queda como pregunta en
`docs/open-questions.md`. El schema del 15/10 es hipótesis hasta la primera
visita.
