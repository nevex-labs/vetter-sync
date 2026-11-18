# Contrato de ingesta Vetter

> Generado por `veterinaria-gachen/scripts/vetter-contract.mjs`. **No editar a
> mano**: se pisa. Regenerar en `veterinaria-gachen` con `pnpm vetter:contract`.
>
> Origen: `veterinaria-gachen@b78ec5e55e9f` (2026-11-18).
> Versión del contrato: **2**.

Este documento es lo que el agente de la PC necesita. El mapeo de las filas
(`tutors`, `pets`, `visits`, `preventives`) vive en el Worker, no acá: el
agente manda las filas crudas de la lista blanca y el hash.

## Endpoint

```
POST /api/sync/vetter
GET  /api/sync/vetter
```

- **Auth**: `Authorization: Bearer <secreto de instalación>`. El secreto lo
  crea un superadmin en el panel (Equipo → Dispositivos) y se muestra una sola
  vez. Empieza con `vgs_`, es revocable y no sirve para el panel.
  Sin el header, o con uno inválido o revocado, responde `401`.
- **GET** es público y devuelve `{ "ok": true, "contract": 2 }`.
  Sirve para verificar desde la PC que el endpoint está vivo, sin credencial.
- **Content-Type**: `application/json`.

## Lote

```json
{
  "rows": [
    {
      "tabla": "PACIENTES",
      "vetter_id": "1524/2",
      "hash": "9f2c…",
      "deleted": false,
      "data": { "NOMBRE": "Toby", "ESPECIE": "Canino" }
    }
  ]
}
```

- Hasta **200 filas** por request y **1.000.000 bytes** de body.
- `data` es un **objeto** JSON: las columnas del DBF tal como vienen, ya
  decodificadas a UTF-8. No se manda el nombre de la tabla adentro de `data`.
- `deleted`: `true` si el registro está marcado como borrado en el DBF.
- `hash`: ver [Hash](#hash). Entre 1 y 128 caracteres.
- `agent_error` (opcional): lo que falló en la PC antes de armar el lote
  (DBF bloqueado, sin red). Se guarda truncado y se ve en el panel.

### Lista blanca

Solo se aceptan estas tablas. Cualquier otra fila se **rechaza y se cuenta**;
no tira el lote entero.

- `CLIENTES`
- `PACIENTES`
- `VACUNAS`
- `VISITAS`
- `ADJUNTOS`

## Respuesta

```json
{
  "received": 0,
  "unchanged": 0,
  "changed": 0,
  "duplicated": 0,
  "applied": 0,
  "ambiguous": 0,
  "rejected": 0
}
```

Claves: `received`, `unchanged`, `changed`, `duplicated`, `applied`, `ambiguous`, `rejected`.

- `received`: filas del body.
- `unchanged`: filas válidas que no cambiaron de hash ni de `deleted`.
- `changed`: filas insertadas o actualizadas.
- `duplicated`: filas repetidas dentro del lote (gana la última).
- `applied`: filas proyectadas al dominio. **En el paso 1 siempre 0.**
- `ambiguous`: filas que no se pudieron enlazar sin adivinar. **En el paso 1
  siempre 0.**
- `rejected`: filas inválidas o de tabla fuera de la lista blanca.

Cierra siempre: `received = unchanged + changed + duplicated + rejected`.

Si el mismo lote se manda dos veces, la segunda devuelve todo en `unchanged` y
no escribe nada. **No se puede usar `applied` para saber si el lote llegó**:
para eso está `received`.

## Heartbeat

Toda request autenticada actualiza la instalación: último contacto, últimos
conteos y último error. Un lote vacío también cuenta: es cómo la PC dice
"estoy vivo" cuando no hay cambios. Un 401 no escribe nada.

## Códigos

| Código | Cuándo | Qué hace el agente |
|---|---|---|
| `200` | Lote procesado, con o sin rechazos | Leer los conteos; si hay `rejected`, revisar el log del Worker |
| `400` | Body que no es JSON, o más de 200 filas | No reintentar: el lote está mal armado |
| `401` | Falta el secreto, no existe o está revocado | No reintentar: revisar el pareo o pedir un dispositivo nuevo |
| `413` | Body de más de 1.000.000 bytes | Partir el lote y reintentar |
| `500` | Falló D1 | Reintentar con backoff |

## Tabla de llegada

Es lo único que el Worker guarda en el paso 1. `data_json` es el `data` de la
fila. `received_at` es la última vez que **cambió el contenido**, no la última
vez que el agente lo mandó.

```sql
CREATE TABLE vetter_rows (
  tabla TEXT NOT NULL,
  vetter_id TEXT NOT NULL,
  hash TEXT NOT NULL,
  data_json TEXT NOT NULL,
  deleted INTEGER NOT NULL DEFAULT 0 CHECK (deleted IN (0, 1)),
  received_at TEXT NOT NULL,
  applied_at TEXT,
  status TEXT NOT NULL DEFAULT 'pendiente'
    CHECK (status IN ('pendiente', 'aplicado', 'ambiguo')),
  PRIMARY KEY (tabla, vetter_id)
) WITHOUT ROWID;
```

## Hash

El servidor no calcula el hash: lo usa para decidir si la fila cambió. Dos
versiones del agente tienen que canonicalizar **igual**, o la primera corrida
después de actualizar reescribe toda la tabla.

Regla v1:

1. Una fila del DBF se convierte en un objeto con los nombres de campo del DBF,
   recortando los espacios de relleno de los campos carácter.
2. Los valores van tipados: texto como string, números como number, fechas como
   `"AAAA-MM-DD"`, lógicos como `0`/`1`. Un campo vacío es `null`, nunca
   `""`.
3. Se agrega `{"deleted": 0|1}` al objeto antes de hashear.
4. Se serializa con las claves **ordenadas alfabéticamente**, sin espacios.
5. `sha256` de ese UTF-8, en hexadecimal minúscula.

Si el encoding del DBF es CP850 o 1252, la decodificación a UTF-8 pasa **antes**
del hash: el hash nunca se calcula sobre bytes crudos.

Cualquier cambio en esta regla es un cambio de contrato: sube
`CONTRACT_VERSION` y obliga a un volcado completo.
