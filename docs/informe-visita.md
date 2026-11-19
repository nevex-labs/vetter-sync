# Informe de la visita — PC de la clínica

> Lo llena Claude Code en la PC y se commitea acá. Sin valores de campos: solo
> estructura, conteos y agregados.

- Fecha: 2026-11-18
- PC / usuario: PC de la clínica (Windows 11 Pro) / `usuario`, `Z:` → `\\Srvarchvete01\c\vetter5`
- Share: `\\Srvarchvete01\c\vetter5\Datos`
- Endpoint: `/api/sync/vetter` · `contract: 3` (`GET` 200 en 2,3 s, sin firewall)
- Inspector: `vetter-agent 0.1.1` (sha256 `44FF5A28…2805`)

## Tablas

75 DBF, 0 errores. Todas Visual FoxPro (`0x30`, con los 263 bytes de backlink en
el header) y language driver `0x03`.

| DBF | registros | borrados | B/reg | codepage | memo | header = tamaño |
|---|---|---|---|---|---|---|
| `CLIENTES.DBF` | 3573 | 37 | 933 | CP1252 | no | sí |
| `PACIENTES.DBF` | 5160 | 46 | 497 | CP1252 | no | sí |
| `VACUNAS.DBF` | 3856 | 101 | 337 | CP1252 | no | sí |
| `visitas.dbf` (minúscula) | 38714 | 0 | 362 | CP1252 | no | sí |
| `ADJUNTOS.DBF` | 3209 | 94 | 536 | CP1252 | no | sí |

Campos (de `fields`):

- `CLIENTES`: `CODIGO N(6)`, `NOMBRE C40`, `DIRECCION C100`, `TELEFONO C40`,
  `SMS C40`, `CIA_CEL C50`, `LOCALIDAD`, `PROVINCIA`, `CODPOSTAL`, `FECHAALTA D`,
  `ULTIMAVEZ D`, `TIPO`, `EMAIL`, `FOTO C254`, `SALDO N(15,4)`, `DATOS_FAC C200`,
  `ANULADO L`.
- `PACIENTES`: `CODIGO N(6)`, `CODIGOPACI C9`, `NOMBRE`, `ESPECIE`, `RAZA`,
  `SEXO`, `FECHA_NAC D`, `ULTIMA_VES D`, `MICROCHIP`, `FOTO`, `VIVE L`,
  `FECHAALTA D`, `PELAJE`, `ANULADO L`, `EXTRAV L`.
- `VACUNAS`: `ID_VACUNA N(10)`, `CODIGOPACI C9`, `VACUNA C80`, `FECHA_APLI D`,
  `FECHA_REVA D`, `MARCA C80`, `REVACUNA C80`, `ES_VACUNA L`, `CANCELADA L`,
  `AVISADO L`, `FORMA_AVIS C50`, `FECHA_AVIS D`.
- `visitas`: `FECHA T` (DateTime), `CODIGO N(10)`, `CLIENTE C40`,
  `CODIGOPACI C9`, `PACIENTE C40`, `DETALLE C254`.
- `ADJUNTOS`: `ID_ADJ N(10)`, `CODIGOPACI C9`, `FECHA D`, `TITULO C254`,
  `ARCHIVO C254`.

### Keys (`config.keys`)

Medidas con `sync --dry-run` (repetidos y vacíos, sin valores):

| Tabla | key | filas | ids repetidos | sin id |
|---|---|---|---|---|
| `CLIENTES` | `CODIGO` | 3573 | 0 | 0 |
| `PACIENTES` | `CODIGOPACI` | 5160 | 3 | 0 |
| `VACUNAS` | `ID_VACUNA` | 3857 | 0 | 0 |
| `VISITAS` | `CODIGO+FECHA` | 38716 | 3 | 0 |
| `ADJUNTOS` | `ID_ADJ` | 3209 | 0 | 0 |

`VISITAS` no tiene id propio: `CODIGO` es el **código de cliente** (3589
valores distintos en 38716 filas; 38415 existen en `CLIENTES`). `CODIGOPACI`
está vacío en 3589 visitas, así que no sirve como parte de la key. `FECHA` es
DateTime al segundo y casi única; `CODIGO+FECHA` da id a todas y deja 3
colisiones (gana la última). Si alguien corrige la fecha de una visita en
Vetter, cambia su `vetter_id`: el Worker la va a ver como fila nueva.

## `CLIENTES.TELEFONO`

- Con datos: 3532 de 3536 vivos
- Solo dígitos: 66
- Largos: 11 → 2340, 13 → 989, 10 → 72, 12 → 36, 21 → 28, 25 → 33, resto < 15
- Separadores: espacio 2642, `+` 1031, `/` 71
- Prefijos (antes de `/`): 12 → 63, 13/14 → 2 c/u, 11/15/16/17 → 1 c/u

## `PACIENTES.CODIGOPACI`

- Con datos: 5114 de 5114 vivos
- Con `/`: 5114
- Largos del prefijo: 1 → 11, 2 → 203, 3 → 1520, 4 → 3380

## Respuestas

1. **¿De dónde abre Vetter? — de OneDrive, y hay dos bases vivas.** El proceso
   `vetter5.exe` de esta PC corre desde `C:\Users\usuario\OneDrive\BKP Vetter5`
   (iniciado 2026-11-15). Los DBF de `OneDrive\BKP Vetter5\Datos` y los del UNC
   están **los dos abiertos para escritura** (una apertura que niega escritura
   falla en ambos). La copia de OneDrive tiene más registros que el UNC
   (`CLIENTES` +2, `PACIENTES` +4, `visitas` +6, `ADJUNTOS` +11; `VACUNAS`
   igual) y el UNC tiene escrituras más nuevas (14:23 vs 13:35): divergen.
   `BKP Vetter.bat` (de otra máquina, usuario `veter`) hace
   `robocopy C:\vetter5 → OneDrive\BKP Vetter5 /MIR`: la próxima corrida pisa
   lo que esta PC cargó en OneDrive. **Hay que decidir la fuente de verdad
   antes del primer sync real.** El agente quedó configurado sobre el UNC,
   como dicen las reglas; no se tocó la configuración de Vetter ni OneDrive.
2. ¿El UNC es accesible con la cuenta de la tarea? Con `usuario`, sí (inspect y
   dry-run leen las 5 tablas). La cuenta de servicio todavía no existe: falta
   probar `Test-Path` con ella.
3. Codepage real: CP1252 (`0x03`) en los 75 DBF.
4. **Marca borrados** con `0x2A`: `CLIENTES` 37, `PACIENTES` 46, `VACUNAS` 101,
   `ADJUNTOS` 94. `visitas` tiene 0 en 38716: o nunca se borran visitas o esa
   tabla se compacta. En la carpeta de Vetter hay un `reindex.exe`. Queda
   abierto: comparar entre dos corridas si desaparecen filas (el agente cuenta
   `desaparecidas`).
5. **Sí.** El prefijo de `CODIGOPACI` es igual a `PACIENTES.CODIGO` en 5114/5114
   y existe en `CLIENTES.CODIGO` en 5114/5114. Hay 13 clientes sin pacientes.
   3 `CODIGOPACI` repetidos (el agente prioriza la fila viva sobre la borrada).
6. Formato de teléfonos: texto libre, mayoría 11 o 13 caracteres con espacio, 1031
   con `+` (formato internacional), 71 con `/` (dos o más números en el campo).
   Cuántos pasan `toWhatsAppPhone` se mide en el Worker.
7. La nota de la visita es el campo `DETALLE C(254)` de `visitas.dbf`. No hay
   `.FPT` en ninguna tabla de la lista blanca.
8. **Sí, mezcla.** `ES_VACUNA`: 3381 `T` y 375 `F` (vivas). Los `F` son lo que
   no es vacuna (antiparasitarios u otros): el Worker tiene que agrupar por
   `ES_VACUNA` + `VACUNA`. `CANCELADA` es `F` en todas.
9. `ADJUNTOS`: `ID_ADJ` (id), `CODIGOPACI` (llave al **paciente**, no a la
   visita; 3115/3115 existen en `PACIENTES`), `FECHA`, `TITULO`, `ARCHIVO`.
   `ARCHIVO` es siempre una ruta relativa con `\`. Extensiones: jpg 1450, pdf
   1264, jpeg 314, mp4 60, docx 8, doc 3, zip 2, png 1, xls 1, otras 12.
10. Sí: el dry-run copió las 5 tablas mientras Vetter las tenía abiertas
    (`os.Open` de Go comparte lectura y escritura) y la copia es consistente
    (header = tamaño). No se probó con `.FPT`: no hay memos en la lista blanca.
11. Sin observar el arranque (no se reinició Vetter en horario de atención). El
    `vetter5.exe` local no tenía conexiones TCP; `SQLTEST.DBF` tiene 1 registro,
    modificado 2026-11-10.
12. Tamaño máximo de `data` (JSON, medido sobre la copia): `CLIENTES` 431 B
    (promedio 353), `PACIENTES` 323, `VACUNAS` 260, `VISITAS` 189, `ADJUNTOS`
    189. Un lote de 200 filas queda muy lejos del 1 MB; los chunks de 90 KB y
    el `WITHOUT ROWID` de `vetter_rows` no necesitan cambios.

## Primer sync (2026-11-18)

Con ok escrito del veterinario (Ley 25.326) y fuente = UNC.

- Pareo: `pair --gui`, secreto cifrado con DPAPI LocalMachine en
  `C:\ProgramData\vetter-agent\secret.bin` (ACL: SYSTEM, Administradores,
  `usuario`). El secreto no pasó por el chat ni por el repo.
- Volcado inicial: 12 corridas de hasta 5000 filas, **54.509 filas en 277
  lotes, 0 `rejected`**, 0 errores. Confirmadas por tabla: `CLIENTES` 3573,
  `PACIENTES` 5157, `VACUNAS` 3857, `VISITAS` 38713, `ADJUNTOS` 3209.
- Corrida de control: 0 lotes, las 5 tablas "sin cambios", heartbeat enviado.
- Tarea programada `Vetter Sync`: cada 15 minutos y al iniciar sesión,
  `IgnoreNew`, reintentos 1 min ×3, límite 10 min. Corre como `usuario` con
  sesión iniciada (usa sus credenciales de red para el UNC, sin guardar
  contraseña) con `C:\ProgramData\vetter-agent\vetter-agent-task.exe` (mismo
  código, compilado sin consola). Primera corrida de la tarea: código 0.

## Observaciones para el release

1. **Dos bases de Vetter vivas (crítico).** El Vetter de esta PC escribe en
   `OneDrive\BKP Vetter5\Datos`; el agente lee el UNC. Lo que se cargue solo en
   esta PC no llega al panel, y el `robocopy /MIR` de `BKP Vetter.bat` lo pisa.
   Hay que apuntar el Vetter de esta PC a `Z:` y sacar el `/MIR` sobre una
   carpeta donde se trabaja.
2. **`VISITAS` no tiene id.** Key `CODIGO+FECHA` (cliente + DateTime): 3
   colisiones en 38.716 y, si se edita la fecha, la visita entra como nueva.
   3589 visitas sin `CODIGOPACI` y 460 con un `CODIGOPACI` que no existe en
   `PACIENTES`: el Worker tiene que enlazarlas por `CODIGO` (cliente).
3. **`PACIENTES` con 3 `CODIGOPACI` repetidos.** El agente manda la fila viva
   sobre la borrada.
4. **`VACUNAS` mezcla tipos:** 375 con `ES_VACUNA = F`. El mapeo a
   `preventives.tipo` tiene que usar ese campo.
5. **`visitas` sin borrados en 38.716 filas.** Sin confirmar si Vetter
   compacta. El agente loguea `desaparecidas` por tabla en cada corrida.
6. **La tarea depende de la sesión de `usuario`.** Si la PC reinicia y nadie
   inicia sesión, no sincroniza hasta el login. Para correr sin sesión hace
   falta una cuenta con contraseña guardada (`C:\ProgramData\vetter-agent\instalar-tarea.ps1`).
7. **Formato de `T`:** `visitas.FECHA` viaja como `"AAAA-MM-DDTHH:MM:SS"`, hora
   local sin zona. El contrato v1 solo define `D`: conviene escribirlo.
8. **`install_id`** viaja en el `User-Agent`; el contrato 3 no tiene campo.
9. **Pendientes del agente:** `status`, auto-update, y que el release genere
   el binario sin consola para la tarea. `inspect` muestra `updated_at` como
   1926 (Visual FoxPro guarda `YY`).

## Errores del inspector

- Ninguno en `errors`.
- `header.updated_at` sale `1926-…`: Visual FoxPro guarda el año como `YY` y el
  inspector suma 1900. No afecta al sync; no se tocó `inspect`.

## JSON

`inspector.json` commiteado en `docs/evidencia/vetter-inspector-2026-11-18.json`.
