# Vetter 5 — el DBF, confirmado

Medido contra la copia del UNC el 2026-11-18 con `inspect` (`vetter-agent 0.1.1`)
y con el primer volcado completo. La evidencia está en
`docs/informe-visita.md` y `docs/evidencia/vetter-inspector-2026-11-18.json`.

## Estructura

- Vetter 5.11.0, Visual FoxPro 7. DBF sin memos en la lista blanca: no hay `.FPT`,
  así que copiar cada archivo alcanza.
- Producción en `\\Srvarchvete01\c\vetter5` (unidad `Z:`), no en la PC. El
  Vetter de la PC escribe en OneDrive: ver `open-questions.md` #1.
- Sin fecha de modificación por fila. La detección de cambios es por hash.
- Encoding: **CP1252** (`0x03`) en los 75 DBF. La decodificación a UTF-8 pasa
  **antes** del hash.
- Borrado lógico: marca el primer byte con `0x2A`, no compacta. `visitas.dbf`
  tiene 0 borrados en 38.716.

## Tablas de la lista blanca

Los tipos son los del header: `C` carácter, `N` numérico, `D` fecha, `L` lógico,
`T` fecha y hora. Las claves de sync están en `config.keys`.

### `CLIENTES` — 3.573 registros, 37 borrados

| Campo | Tipo | Uso |
|---|---|---|
| `CODIGO` | N(6) | `tutors.vetter_id`, clave de sync |
| `NOMBRE` | C(40) | `tutors.first_name` (apellido primero, ver ADR 0020) |
| `TELEFONO` | C(40) libre | `tutor_phones`; puede traer varios números |
| `EMAIL`, `DIRECCION`, `LOCALIDAD` | C | no se proyectan |
| `ANULADO` | L | 0 en las 3.573; la proyección lo parkea si viene en 1 |
| `SALDO`, `FOTO`, `DATOS_FAC` | — | fuera de alcance |

`TELEFONO` se separa y normaliza a formato Meta (`549` + área + número). Los
inválidos se cuentan y se descartan; los compartidos no se asignan a nadie.

### `PACIENTES` — 5.160 registros, 46 borrados

| Campo | Tipo | Uso |
|---|---|---|
| `CODIGOPACI` | C(9), ej. `1524/2` | `pets.vetter_id`, clave de sync |
| `CODIGO` | N(6) | cliente; el prefijo de `CODIGOPACI` da lo mismo en 5.114/5.114 |
| `NOMBRE` | C | `pets.name` |
| `ESPECIE`, `RAZA` | C | `pets.species`, `pets.breed` |
| `SEXO` | C | `pets.sex` + `pets.neutered` (6 valores, ver ADR 0020 §5) |
| `FECHA_NAC` | D | `pets.birth_date`, validada 1980..hoy |
| `VIVE` | L | `pets.alive`: 1 vivo, 0 no vivo |
| `ANULADO`, `EXTRAV` | L | 0 y 2 filas respectivamente; sin semántica confirmada |
| `MICROCHIP`, `PELAJE`, `FOTO` | — | no se proyectan |

3 `CODIGOPACI` repetidos: viajan los dos con `row_key` y la repetida queda
`ambiguo`.

### `VACUNAS` — 3.856 registros, 101 borrados

| Campo | Tipo | Uso |
|---|---|---|
| `ID_VACUNA` | N(10) | `preventives.vetter_id`, clave de sync |
| `CODIGOPACI` | C(9) | paciente |
| `VACUNA` | C(80) | `preventives.product` |
| `ES_VACUNA` | L | `preventives.type`: `T` vacuna, `F` antiparasitario |
| `FECHA_APLI` | D | `preventives.date`; 15 sin fecha válida |
| `FECHA_REVA` | D | `preventives.next_dose`, si cae entre hoy y +3 años |
| `CANCELADA` | L | `preventives.cancelled_at` |
| `AVISADO`, `FECHA_AVIS` | L, D | `preventives.reminder_sent_at`; hoy en 0 |

Vivas: 3.381 vacunas y 375 antiparasitarios. `CANCELADA` está en `F` en todas.

### `visitas` — 38.716 registros, 0 borrados

| Campo | Tipo | Uso |
|---|---|---|
| `FECHA` | T | fecha y hora del evento |
| `CODIGO` | N(10) | **código de cliente** (3.589 valores distintos) |
| `CODIGOPACI` | C(9) | paciente; vacío en 3.589 eventos (`NUEVO CLIENTE`) |
| `DETALLE` | C(254) | tipo de evento o nota |

No hay campo de profesional. Es una bitácora de eventos, no una lista de
atenciones: **no se proyecta** (ADR 0020). Clave de sync `CODIGO+FECHA`; 3
colisiones.

### `ADJUNTOS` — 3.209 registros, 94 borrados

| Campo | Tipo | Uso |
|---|---|---|
| `ID_ADJ` | N(10) | clave de sync |
| `CODIGOPACI` | C(9) | **paciente**, no visita (3.115/3.115 existen) |
| `FECHA`, `TITULO` | D, C | metadata |
| `ARCHIVO` | C | ruta relativa con `\` |

Los archivos no se suben todavía: la metadata sola no se proyecta.

## Lo que el agente manda

Las filas crudas con el nombre de campo del DBF en `data`, más `tabla`,
`vetter_id`, `row_key`, `hash` y `deleted`. No manda el mapeo.

El contrato exacto está en `contract.md`, que se genera desde el Worker.
