# Preguntas de la visita — respuestas del 2026-11-18

Contestadas contra la copia del UNC con `vetter-agent 0.1.1` (`inspect` +
`sync --dry-run`), no de memoria. La evidencia está en
`docs/informe-visita.md` y `docs/evidencia/vetter-inspector-2026-11-18.json`.

## Contestadas

1. **¿De dónde abre Vetter? — De OneDrive, y hay dos bases vivas.** El
   `vetter5.exe` de esta PC corre desde `C:\Users\usuario\OneDrive\BKP Vetter5`
   y escribe ahí. Los DBF del UNC y de OneDrive están **los dos abiertos para
   escritura** y divergen: OneDrive tiene más registros (`CLIENTES` +2,
   `PACIENTES` +4, `visitas` +6, `ADJUNTOS` +11; `VACUNAS` igual) y el UNC tiene
   escrituras más nuevas (14:23 contra 13:35). `BKP Vetter.bat`, de otra
   máquina, hace `robocopy C:\vetter5 → OneDrive\BKP Vetter5 /MIR`: la próxima
   corrida pisa lo que esta PC cargó en OneDrive. **Ver el punto 1 de “sigue
   abierto”.**
2. **¿El UNC es accesible con la cuenta de la tarea?** Con `usuario`, sí.
   La cuenta de servicio todavía no existe.
3. **Encoding**: CP1252 (`0x03`) en los 75 DBF.
4. **Borrados**: marca con `0x2A`, no compacta. `CLIENTES` 37, `PACIENTES` 46,
   `VACUNAS` 101, `ADJUNTOS` 94, `visitas` 0 de 38.716.
5. **`CODIGOPACI`**: el prefijo es igual a `PACIENTES.CODIGO` y existe en
   `CLIENTES.CODIGO` en **5.114/5.114**. 13 clientes no tienen pacientes.
6. **Teléfonos**: 3.532 de 3.536 vivos con datos; 2.340 de 11 dígitos, 989 de
   13, 72 de 10, 36 de 12; separadores espacio (2.642), `+` (1.031) y `/` (71);
   71 campos con más de un número.
7. **Nota de la visita**: `DETALLE C(254)` en `visitas.dbf`. No hay `.FPT` en
   ninguna tabla de la lista blanca.
8. **`VACUNAS` mezcla tipos**: `ES_VACUNA` 3.381 `T` y 375 `F` (vivas);
   `CANCELADA` es `F` en todas.
9. **`ADJUNTOS`**: `ID_ADJ`, `CODIGOPACI` (llave al **paciente**, no a la
   visita: 3.115/3.115 existen en `PACIENTES`), `FECHA`, `TITULO`, `ARCHIVO`
   (ruta relativa con `\`).
10. **¿Se puede copiar con Vetter abierto?** Sí: las 5 tablas se copiaron con
    Vetter en uso y quedaron consistentes (header = tamaño).
11. **¿Qué hace Vetter al arrancar con la red?** No se observó: no se reinició
    Vetter en horario de atención. `SQLTEST.DBF` tiene 1 registro.
12. **¿Cuánto pesa una fila?** Máximo 431 B (`CLIENTES`), promedio ~350 B. Un
    lote de 200 filas queda lejos del 1 MB; los chunks de 90 KB y el
    `WITHOUT ROWID` no cambian.

## Sigue abierto

1. **Fuente de verdad (crítico).** El agente lee el UNC y el Vetter de esta PC
   escribe en OneDrive. Lo que se cargue solo en esta PC no llega al panel, y
   el `robocopy /MIR` puede pisarlo. Propuesta de la visita: apuntar el Vetter
   de esta PC a `Z:` y sacar el `/MIR` sobre una carpeta donde se trabaja.
   Mientras no se decida, el import corre sobre el UNC y puede faltarle lo que
   solo está en OneDrive (2 clientes, 4 pacientes, 6 visitas, 11 adjuntos).
2. **Cuenta de servicio** para que la tarea corra sin sesión iniciada. Hoy
   depende de que `usuario` haya iniciado sesión; si la PC reinicia y nadie
   entra, no sincroniza.
3. **Colisiones de `PACIENTES` — resuelto con `row_key`.** Tres pares comparten
   `CODIGOPACI`. Desde el agente 0.2.2 las dos filas viajan (`id` y `id#2`) y el
   Worker las guarda a las dos; la repetida queda `ambiguo` con
   `reason = 'duplicado'`. Los 3 que se perdieron en el primer volcado vuelven
   solos en la primera corrida con 0.3.0.
4. **Pacientes que la bitácora nombra y no existen.** 306 eventos de `VISITAS`
   sobre 56 códigos de paciente que no están en `PACIENTES` (ni vivos ni
   borrados); 40 de esos clientes sí existen y 20 tienen otros pacientes con la
   numeración salteada. Consistente con haberse borrado con `PACK`; falta
   confirmarlo contra el DBF.
5. **¿Vetter compacta `visitas`?** 0 borrados en 38.716. El agente loguea
   `desaparecidas` por tabla en cada corrida: dos corridas alcanzan.
6. **`visitas.FECHA` viaja como `T`** (`AAAA-MM-DDTHH:MM:SS`, hora local sin
   zona). El contrato ya lo documenta; falta que el Worker lo use si algún día
   se proyectan las visitas.
