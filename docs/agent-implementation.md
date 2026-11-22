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
  // Opcional: carpeta donde escribe el Vetter de esta PC. Si está, cada corrida
  // compara la cantidad de registros de cada tabla contra el UNC y avisa por
  // agent_error si divergen. Abre solo headers: no importa datos de la réplica.
  // Cantidades iguales no significan copias iguales (VACUNAS ya divergía con
  // el mismo número); el silencio no es un "está sincronizado".
  "mirror_path": "C:\\Users\\usuario\\OneDrive\\BKP Vetter5\\Datos",
  "max_rows_per_run": 5000,
  "keys": {
    "CLIENTES": "CODIGO",
    "PACIENTES": "CODIGOPACI",
    "VACUNAS": "ID_VACUNA",
    "VISITAS": "CODIGO+FECHA",
    "ADJUNTOS": "ID_ADJ"
  }
}
```

Keys confirmadas en la visita del 2026-11-18 (`docs/informe-visita.md`). Si
falta una, `sync` no adivina: la saltea y reporta `agent_error`.

- Una key compuesta (`"A+B"`) une los valores con `|`:
  `"1524|2026-10-14T09:00:00"`. Si falta alguna parte, la fila no tiene id y se
  cuenta como `sin_id`. `VISITAS` la necesita: no tiene id propio y su
  `CODIGO` es el del cliente.
- Si un `vetter_id` se repite en la tabla gana la última, salvo que la última
  esté borrada y la anterior viva: un borrado viejo no anula una fila viva.
- `log_dir` es opcional; por defecto `logs\` al lado del snapshot.
- El `install_id` viaja en el `User-Agent` (`vetter-agent/<versión> (pc-vete)`):
  el contrato 3 no tiene un campo para eso en el body.

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
- `T` (DateTime de Visual FoxPro, `visitas.FECHA`): `"AAAA-MM-DDTHH:MM:SS"`,
  hora local de la clínica sin zona, redondeado al segundo; vacío → `null`.
- `I`, `B`, `Y`: entero, `float64` y moneda (`float64`). La columna de sistema
  `_NullFlags` (`0`) no se manda.
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
- `vetter-agent status --config ...` (todavía no existe) debería imprimir, sin
  red: si el secreto se puede leer, último run, tablas pendientes, tamaño del
  outbox y último error local. Es lo primero que mira Claude Code cuando algo no
  anda; hoy se lee con `Get-ScheduledTaskInfo` y el log.
- Logs en `C:\ProgramData\vetter-agent\logs\agent-AAAA-MM-DD.log`, una línea
  JSON por evento (`ts`, `level`, `msg`, `tabla`, `filas`, `vetter_id`).
  Rotación diaria y borrado a los 7 días. Nunca PII.
- Cada corrida hace un chequeo previo: secreto legible, UNC alcanzable,
  `GET` del endpoint. Si falla, loguea, intenta mandar `agent_error` y sale
  con código 1. No se queda reintentando en silencio.
- Un error del servidor (400, 413, 500) se loguea con el código; el panel
  además lo ve por el heartbeat. El 401 corta la corrida.

## Auto-update

Objetivo: publicamos un release desde casa y la PC se actualiza sola, sin que
nadie la toque ni abra Claude Code.

- Manifiesto: `https://github.com/nevex-labs/vetter-sync/releases/latest/download/manifest.json`
  con `{version, url, sha256, published_at, min_contract}`.
- Firma: ed25519 sobre los bytes exactos del manifiesto, en `manifest.sig`. La
  clave pública se compila con `-X main.updatePublicKey=...`; si está vacía,
  auto-update deshabilitado (falla cerrado).
- Chequeo: una vez cada 24 h, al principio del sync. La marca vive en
  `update.checked`, al lado del snapshot; si no se puede consultar (sin red, sin
  release) se sigue sincronizando.
- Descarga a `<exe>.new`, al lado del binario, y verifica **firma y sha256**
  antes de tocar nada: el `.new` se borra si el hash no coincide.
- Swap en la misma corrida del chequeo: se renombra el ejecutable en curso a
  `.old` y el `.new` ocupa su nombre. Windows permite renombrar un `.exe` en
  ejecución; el proceso actual termina con el código que ya tiene cargado y la
  corrida siguiente arranca con la versión nueva.
- Rollback: cada corrida suma uno en `update.attempts`; una corrida que termina
  bien lo pone en cero. Con 3 y un `.old` presente, se restaura el anterior y se
  loguea. Cubre el caso de una versión que arranca pero no sincroniza; no cubre
  un binario que no llega a ejecutarse, que la verificación de sha256 hace
  improbable.
- Contrato: si `min_contract` del release es mayor que el del agente, no se
  instala y se loguea; el release se puede publicar igual.
- Logs (en español, como el resto del agente): `update: no se pudo consultar el
  release`, `update: manifiesto rechazado`, `update: no se pudo bajar la
  versión`, `update: instalada, la próxima corrida la usa`, `update: la versión
  nueva falló tres veces, restaurada la anterior`.
- `status` (versión actual, último chequeo y fallas) sigue pendiente.
- El heartbeat manda `agent_version`; el panel muestra qué versión corre cada
  PC. Ese campo va en el contrato (paso siguiente en gachen).

## Tests

- DBF sintéticos (no de la clínica): header, codepage, borrados, fechas.
- Los tres vectores de hash.
- Dos corridas seguidas: la segunda manda 0 filas.
- `deleted` viaja como `true` y el `vetter_id` sale de `keys`.
- `413` parte el lote; `401` corta.
- `go test ./...` y `go vet ./...`.

## Estado (2026-11-22)

Implementado: `pair` (consola con `*`, `--clipboard` y `--gui`), `sync` y
`sync --dry-run`, con copia, parser, hash, snapshot, lotes, 413/401/400/500,
outbox, heartbeat, `agent_error` y logs diarios. También `setup` (la ventana de
instalación), `row_key` para las filas repetidas, el aviso de copias
divergentes (`mirror_path`) y el auto-update firmado con rollback.

En la PC: pareado, volcado inicial completo y tarea `Vetter Sync` cada 15
minutos (ver `informe-visita.md`). Pendiente: el comando `status`, y llevar la
PC de 0.1.1 a la última versión con `vetter-setup.exe` (issue #2).

## Terminado cuando

- `go build` da un `.exe` que corre en la PC sin dependencias.
- `sync --dry-run` imprime conteos por tabla y no toca la red.
- Una corrida real manda el lote, el panel muestra `last_seen_at` y conteos.
- No hay PII en logs ni en tests.
