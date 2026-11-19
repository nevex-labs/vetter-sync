# Vetter 5 — lo que sabemos del DBF

Todo lo de acá es **hipótesis** hasta confirmarlo contra la copia de `Z:`. La
fuente original es el diagnóstico del 2026-10-15 en
`veterinaria-gachen/docs/product/vetter-hallazgos-2026-10-15.md`, que tampoco se
corrió en local. Cada fila que se confirme se marca acá con fecha y evidencia.

## Estructura

- Vetter 5.11.0, Visual FoxPro 7. Tablas `.DBF` + memo `.FPT`.
- Producción en `\\Srvarchvete01\c\vetter5` (unidad `Z:`), no en la PC.
- Sin fecha de modificación por fila. La detección de cambios es por hash.
- Encoding: por confirmar. El header del DBF trae el language driver (byte 29):
  CP850 y CP1252 son los candidatos. **La decodificación a UTF-8 pasa antes del
  hash.**
- Borrado lógico: por confirmar si Vetter marca el primer byte con `0x2A` o si
  hace `PACK`. Si hace `PACK`, la fila desaparece y no se distingue de "nunca
  existió"; el sync nunca borra, así que quedaría fantasma. Ver
  `open-questions.md`.

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
| `CODIGOPACI` | texto, ej. `1524/2` | `pets.vetter_id` | no |
| `NOMBRE` | texto | `pets.name` | no |
| `ESPECIE` | texto | `pets.species` | no |
| `RAZA` | texto | `pets.breed` | no |
| `SEXO` | texto | pendiente de schema | no |

Hipótesis fuerte: `CODIGOPACI` es `código de cliente / número de paciente`. Si
se confirma, el tutor se puede derivar del código de paciente aunque la fila de
`CLIENTES` no matchee. Es la pregunta más importante de la visita.

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
