# Calidad del agente Go

## Capas, de más fuerte a más débil

1. **Gates determinísticos**: `gofmt`, `go vet`, `golangci-lint` y
   `go test -race`. Corren en CI en cada push y son el comando obligatorio
   antes de dar algo por terminado. No dependen de que el modelo se acuerde.
2. **`.golangci.yml`**: lo que más pesa contra el slop típico (errores
   ignorados, sin `%w`, `context` sin propagar, dependencias de más, `panic`).
3. **`AGENTS.md`**: solo lo que el linter no puede ver (estructura, tests,
   logging, nombres).
4. **Skill**: no se usa. Es la capa más débil y diluye contexto; el repo ya
   tiene la especificación concreta en `docs/agent-implementation.md`.

No hay hook de Claude Code a propósito: en la PC Go puede no estar instalado y
un hook roto bloquea el turno. El gate va en CI y en el comando final del
prompt. Si Go está, correr el comando es una línea.

## Qué se exige y por qué

- `errcheck`, `errorlint`: errores ignorados o sin `%w`.
- `wrapcheck`: no devolver el error crudo de otro paquete; agregar contexto.
- `gosec`: permisos de archivo, conversiones inseguras.
- `noctx`, `bodyclose`, `contextcheck`: HTTP con `context` y body cerrado.
- `depguard` en modo estricto: **stdlib y este módulo, nada más**. Cualquier
  dependencia nueva se justifica en el PR.
- `forbidigo`: `panic` prohibido; se devuelve el error.
- `cyclop`/`dupl`: funciones y bloques repetidos.
- `revive`, `gocritic`: estilo y trampas comunes.
- `paralleltest`: los tests no comparten estado.

`sqlclosecheck` y `rowserrcheck` están porque no molestan; el agente no usa SQL.
`testifylint` también: si algún día entra testify, ya está la regla.

`gox` (el linter orientado a código de LLM) queda como algo a evaluar, no como
estándar: se solapa con `golangci-lint` y no lo probamos.

## Slop que se mira en la revisión

- `util/`, `common/`, `helpers/`.
- Interfaces con una sola implementación.
- Globals e `init()`.
- `fmt.Print` como logging (va `slog`; `fmt` solo en `main` para la CLI).
- Mocks para todo en vez de un DBF sintético.
- `time.Sleep` en tests.
- Goroutines sin forma de terminar.
- Capas estilo Java entre `main` y el parser.
