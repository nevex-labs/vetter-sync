# Vetter 5 — lo que sabemos del DBF

Todo lo de acá es **hipótesis** hasta confirmarlo contra la copia de `Z:`. La
fuente original es el diagnóstico del 2026-10-15 en
`veterinaria-gachen/docs/product/vetter-hallazgos-2026-10-15.md`, que tampoco se
corrió en local. Cada fila que se confirme se marca acá con fecha y evidencia.

## Estructura

- Vetter 5.11.0, Visual FoxPro 7. Tablas `.DBF` + memo `.FPT`.
- Producción en `\\Srvarchvete01\c\vetter5` (unidad `Z:`), no en la PC.
- Sin fecha de modificación por fila. La detección de cambios es por hash.
- Encoding: **CP1252** (`0x03`) en los 75 DBF, confirmado el 2026-11-18 con
  `inspect`. **La decodificación a UTF-8 pasa antes del hash.**
- Borrado lógico: **marca el primer byte con `0x2A`, no compacta**
  (2026-11-18). Igual hay que vigilar `PACK`: si alguna vez corre, la fila
  desaparece y el sync no se entera; el agente loguea `desaparecidas` por
  corrida. Ver `open-questions.md`.

## Tablas de la lista blanca

### `CLIENTES`

| Campo (hipótesis) | Tipo | Uso | Confirmado |
|---|---|---|---|
| `CODIGO` | texto | `vetter_id` del tutor | no |
| `NOMBRE` | texto | nombre del tutor | no |
| `TELEFONO` | texto 40c libre | `tutor_phones` | no |

`TELEFONO` es texto libre y puede traer varios números. El import lo normaliza
con `normalizePhone` y `toWhatsAppPhone` (formato Meta: `549` + área + número).
Los inválidos se cuentan y se descartan.

### `PACIENTES`

| Campo (hipótesis) | Tipo | Uso | Confirmado |
|---|---|---|---|
| `CODIGOPACI` | texto, ej. `1524/2` | `pets.vetter_id` | sí (2026-11-18) |
| `NOMBRE` | texto | `pets.name` | no |
| `ESPECIE` | texto | `pets.species` | no |
| `RAZA` | texto | `pets.breed` | no |
| `SEXO` | texto | pendiente de schema | no |

`CODIGOPACI` es `código de cliente / número de paciente`: el prefijo coincide
con `PACIENTES.CODIGO` y existe en `CLIENTES.CODIGO` en 5.114/5.114
(confirmado el 2026-11-18).

### `VACUNAS`

| Campo (hipótesis) | Uso | Confirmado |
|---|---|---|
| `FECHA_REVA` | próxima dosis → `preventives.next_dose` | no |
| tipo / producto | `preventives.type` y `.product` | no |
| `AVISADO` | no se usa | no |

`preventives.type` solo admite `vacuna` o `antiparasitario`. Si `VACUNAS` mezcla
otras cosas, hay que decidir cómo se agrupan antes de proyectar.

### `VISITAS`

| Campo (hipótesis) | Uso | Confirmado |
|---|---|---|
| fecha / hora | `visits.fecha` y `.hora` | no |
| texto de la atención | `visit_notes.text` (no se proyecta: ADR 0020) | no |
| profesional | `visits.doctor_id` (3 códigos) | no |

Es la única tabla con timestamp por evento. Es una bitácora de eventos, no una lista de atenciones: no se proyecta (ADR 0020). La historia clínica real está en `CLINICA`.

### `ADJUNTOS`

| Campo (hipótesis) | Uso | Confirmado |
|---|---|---|
| ruta / nombre del archivo | R2 `vetter/<tabla>/<vetter_id>/<nombre>` | no |
| vínculo al paciente o la visita | `attachments.pet_id` / `visit_id` | no |
| tipo | `kind`: estudios, radiografías, imágenes | no |

Volumen del reporte: ~1.300 PDF, ~1.789 JPG y un ZIP de 458 MB. El ZIP no se
sube. Los archivos van después del backup de R2 y con `FILES_PER_RUN` bajo.

## Lo que el agente manda

Las filas crudas de estas tablas, con el nombre de campo tal como viene del DBF
en `data`, más `tabla`, `vetter_id`, `hash` y `deleted`. No manda el mapeo.

El contrato exacto está en `contract.md`, que se genera desde el Worker.
