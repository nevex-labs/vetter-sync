# Agente: parser, hash y sync

> Lo que falta implementar en `agent/` después del `inspect`. El contrato de
> transporte está en `contract.md`; las reglas duras, en `AGENTS.md`.

## Alcance

Agregar dos subcomandos, sin tocar `inspect`:

```
vetter-agent pair   [--config <ruta>]
vetter-agent sync   --config <ruta> [--dry-run]
```

`pair` guarda el secreto de instalación una vez. `sync` es la corrida de la
tarea programada: copia, parsea, hashea y empuja. No mapea a `tutors`, `pets`,
`visits` ni `preventives`: eso vive en el Worker.

## Config

`config.json`, fuera del repo (en la PC):

```json
{
  "z_path": "\\\\SERVIDOR-VETTER\\c\\vetter5\\Datos",
  "copy_dir": "C:\\ProgramData\\vetter-agent\\copy",
  "snapshot": "C:\\ProgramData\\vetter-agent\\snapshot.json",
  "endpoint": "https://<worker>/api/sync/vetter",
  "install_id": "pc-vete",
  "max_rows_per_run": 5000,
  "keys": {
    "CLIENTES": "CODIGO",
    "PACIENTES": "CODIGOPACI",
    "VACUNAS": "",
    "VISITAS": "",
    "ADJUNTOS": ""
  }
}
```

Las tres claves vacías se completan con lo que muestre el inspector. Si falta
una, `sync` no adivina: la saltea y reporta `agent_error`.

## Secreto

`pair` pide el secreto por stdin sin eco (nunca por argumento ni por chat) y lo
guarda con DPAPI scope `LocalMachine` en
`%ProgramData%\vetter-agent\secret.bin`, con ACL para la cuenta de la tarea.
DPAPI de usuario no sirve si la tarea corre con S4U o con otra cuenta.

Validación del pareo: un `POST` con `{"rows":[]}`. `200` = listo; `401` =
secreto inválido o revocado.

## Copia

1. Comparar `(size, mtime)` del DBF con el snapshot de archivos. Si no cambió,
   saltear la tabla.
2. Copiar a `copy_dir` con nombre temporal y `rename`. Abrir el original en
   solo lectura y en modo compartido.
3. Volver a mirar `mtime` después de copiar; si cambió, reintentar una vez.
4. Parsear la copia, nunca el original.

## Parser

- Header: versión, registros, largo de header/registro, language driver.
- Decodificar CP850 y CP1252 a UTF-8 con tablas propias (sin dependencias).
- Campos `C`: recortar espacios y `\x00` a la derecha; vacío → `null`.
- `N`/`F`: entero si no tiene decimales, `float64` si tiene.
- `D`: `"AAAA-MM-DD"` o `null`.
- `L`: `0`/`1`.
- `M`: si el inspector muestra que la nota vive en memo, leer el `.FPT`
  (bloque de 64 bytes, tamaño en el header). Si no, dejarlo como número de
  bloque; no inventar texto.
- Borrados: **no se saltean**. Se mandan con `deleted: true` para que el Worker
  anule visitas y preventivos.

## Hash

Regla v1 del contrato. En Go:

```go
payload := map[string]any{...campos...}
payload["deleted"] = 0 // o 1
enc := json.NewEncoder(&buf)
enc.SetEscapeHTML(false)
enc.Encode(payload) // claves ordenadas, sin espacios, sin newline final
sha256.Sum256(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
```

Vectores para testear la implementación:

| data | deleted | sha256 |
|---|---|---|
| `{"FECHA":"2026-10-14","NOMBRE":"Toby","PESO":12.5,"VACIO":null}` | false | `7ad0011f65b051a4155338551a016e8b247b20bd28ee8dd275cdd6f88ee53d7a` |
| `{"NOMBRE":"Ñandú","TUTOR":"Pérez"}` | false | `83772c3537ce0dc66594750f4e49d6a33b0f690717bea54cc5a5494b69db6050` |
| `{"CODIGO":"1524"}` | true | `7cbf9dce123746c03d5478341fb84ef4e9b2b6b10552b941aeb54d15d1fa90b6` |

## Snapshot

Un JSON `{"TABLA|vetter_id": "hash"}` con escritura atómica (temp + rename).
Primera corrida: vacío, manda todo. Las siguientes: solo lo que cambió. Guardar
también `(size, mtime)` por archivo para saltear tablas sin cambios.

## Sync

- Lotes de 200 filas; `max_rows_per_run` reparte el volcado inicial.
- Un lote vacío al final de cada corrida: es el heartbeat.
- `200`: borrar del snapshot solo lo confirmado.
- `401`: cortar la corrida, no reintentar.
- `400`: log y seguir; es un bug del agente, no del servidor.
- `413`: partir el lote a la mitad.
- `500` o red: backoff exponencial (1 s, 2 s, 4 s, hasta 5 min) y outbox local
  en `copy_dir/outbox` para no perder lo pendiente.
- Si falla antes de armar el lote (DBF bloqueado, share caído), mandar
  `{"rows":[],"agent_error":"..."}` con un texto corto y sin PII.

## Operación desatendida

Requisito: **anda solo, siempre, y cuando falla se ve.**

- Tarea programada cada 5 minutos, con trigger de inicio, `MultipleInstances
  IgnoreNew`, `StartWhenAvailable` y reinicio a los 1/2/5 minutos si falla.
  Corre con la cuenta de servicio, sin ventana.
- Códigos de salida: `0` ok, `1` error de configuración/share/secreto, `2` uso
  incorrecto. Task Scheduler guarda el último código.
- `vetter-agent status --config ...` imprime, sin red: si el secreto se puede
  leer, último run, tablas pendientes, tamaño del outbox y último error local.
  Es lo primero que mira Claude Code cuando algo no anda.
- Logs en `C:\ProgramData\vetter-agent\logs\agent-AAAA-MM-DD.log`, una línea
  JSON por evento (`ts`, `level`, `msg`, `tabla`, `filas`, `vetter_id`).
  Rotación diaria y borrado a los 7 días. Nunca PII.
- Cada corrida hace un chequeo previo: secreto legible, UNC alcanzable,
  `GET` del endpoint. Si falla, loguea, intenta mandar `agent_error` y sale
  con código 1. No se queda reintentando en silencio.
- Un error del servidor (400, 413, 500) se loguea con el código; el panel
  además lo ve por el heartbeat. El 401 corta la corrida.

## Tests

- DBF sintéticos (no de la clínica): header, codepage, borrados, fechas.
- Los tres vectores de hash.
- Dos corridas seguidas: la segunda manda 0 filas.
- `deleted` viaja como `true` y el `vetter_id` sale de `keys`.
- `413` parte el lote; `401` corta.
- `go test ./...` y `go vet ./...`.

## Terminado cuando

- `go build` da un `.exe` que corre en la PC sin dependencias.
- `sync --dry-run` imprime conteos por tabla y no toca la red.
- Una corrida real manda el lote, el panel muestra `last_seen_at` y conteos.
- No hay PII en logs ni en tests.
