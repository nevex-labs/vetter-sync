# Informe de la visita — PC de la clínica

> Lo llena Claude Code en la PC y se commitea acá. Sin valores de campos: solo
> estructura, conteos y agregados.

- Fecha:
- PC / usuario:
- Share: `\\Srvarchvete01\c\vetter5\Datos`
- Endpoint: `/api/sync/vetter` · `contract: 2`
- Inspector: `vetter-agent 0.1.0`

## Tablas

| DBF | registros | borrados | B/reg | codepage | memo | header = tamaño |
|---|---|---|---|---|---|---|
| | | | | | | |

## `CLIENTES.TELEFONO`

- Con datos:
- Solo dígitos:
- Largos:
- Separadores:
- Prefijos:

## `PACIENTES.CODIGOPACI`

- Con datos:
- Con `/`:
- Largos del prefijo:

## Respuestas

1. ¿De dónde abre Vetter? (`Z:` vs OneDrive)
2. ¿El UNC es accesible con la cuenta de la tarea?
3. Codepage real:
4. ¿Marca borrados o hace `PACK`?
5. ¿El prefijo de `CODIGOPACI` es el cliente?
6. Formato de teléfonos y cuántos por campo:
7. ¿Dónde está la nota de la visita? (campo o memo `.FPT`)
8. ¿`VACUNAS` incluye antiparasitarios?
9. Estructura de `ADJUNTOS`:
10. ¿Se puede copiar con Vetter abierto?
11. ¿Qué hace Vetter al arrancar con red? (`SQLTEST.DBF`)
12. Tamaño máximo de una fila de `data_json` (se mide en el primer envío):

## Errores del inspector

- 

## JSON

`inspector.json` adjunto / commiteado en:
