# Preguntas abiertas — visita a la PC

Son las que bloquean el parser y el hash. Cada una se contesta contra la copia,
no de memoria. Lo que se cierre se pasa a `vetter-schema.md` con fecha.

## 1. ¿De dónde abre Vetter?

- **Cómo**: ver la ruta del acceso directo y el proceso en ejecución
  (`Get-Process vetter5 | Select-Object Path`); comparar con `Z:` y con
  `OneDrive\BKP Vetter5`.
- **Bloquea**: la fuente de verdad. Si Vetter trabaja sobre la réplica de
  OneDrive, el sync tiene que leer lo que Vetter escribe, no producción.
- **Riesgo**: el próximo `robocopy /MIR` o la sincronización de OneDrive puede
  pisar datos. Es independiente de nosotros.

## 2. ¿El UNC es accesible con la cuenta de la tarea?

- **Cómo**: `Test-Path \\Srvarchvete01\c\vetter5\Datos` con la cuenta de
  servicio.
- **Bloquea**: `z_path`. Confirmar que no depende de una unidad mapeada.

## 3. ¿Qué encoding tiene cada DBF?

- **Cómo**: leer el byte 29 del header (language driver). Si es `0x03`/`0x57`
  u otro, mapear a CP1252/CP850; si es `0x00`, hay que decidirlo por los datos.
- **Bloquea**: la decodificación a UTF-8, que pasa **antes** del hash.

## 4. ¿Vetter marca borrados o hace `PACK`?

- **Cómo**: mirar el primer byte de las filas (¿hay `0x2A`?) y si hay filas que
  desaparecen entre dos copias. Buscar si Vetter tiene una opción de compactar.
- **Bloquea**: el `deleted` que se manda. Si hace `PACK`, el sync nunca se
  entera y las filas quedan fantasma: hay que decidir qué hacer antes de
  proyectar.

## 5. ¿Cómo es `CODIGOPACI`?

- **Cómo**: contar formatos, ver si el prefijo coincide con `CLIENTES.CODIGO`,
  y si hay pacientes sin cliente o clientes sin pacientes.
- **Bloquea**: el `vetter_id` de la mascota y, si se confirma el prefijo, la
  posibilidad de derivar el tutor aunque `CLIENTES` no matchee.

## 6. ¿Cómo vienen los teléfonos?

- **Cómo**: muestrear `TELEFONO` (sin exportar nombres), contar cuántos tienen
  varios números, separadores, y cuántos pasan `toWhatsAppPhone`.
- **Bloquea**: `tutor_phones`, el enlace por teléfono y el recordatorio. Los
  inválidos se descartan y se cuentan.

## 7. ¿Dónde está la nota de la visita?

- **Cómo**: abrir un registro de `VISITAS` y ver si el texto es un campo del
  DBF o un memo `.FPT`.
- **Bloquea**: `visit_notes` y la regla de deduplicación del paso 4.

## 8. ¿`VACUNAS` incluye antiparasitarios?

- **Cómo**: ver los valores de tipo/producto.
- **Bloquea**: el `CHECK (tipo IN ('vacuna','antiparasitario'))` de
  `preventives`. Si hay otros tipos, hay que agruparlos o ampliar el schema.

## 9. ¿Cómo se arma la ruta de un adjunto?

- **Cómo**: ver la estructura de `ADJUNTOS` y un archivo real: nombre, carpeta,
  llave al paciente o a la visita.
- **Bloquea**: el layout de R2 y el `attachments.pet_id`. Sin llave, el adjunto
  entra huérfano como los 44 de hoy.

## 10. ¿Se puede copiar con Vetter abierto?

- **Cómo**: copiar un DBF mientras Vetter está en uso y verificar que la copia
  abre. Repetir con el `.FPT` de una tabla con memos.
- **Bloquea**: la estrategia de copia. Si el share no permite lectura
  concurrente, hay que copiar en una ventana o con reintentos.

## 11. ¿Qué hace Vetter al arrancar con la red?

- **Cómo**: `Get-NetTCPConnection` corriendo mientras se abre Vetter, y mirar
  `SQLTEST.DBF`.
- **Bloquea**: nada del sync, pero cierra la duda de licencia/instancia del
  `SPEC.md` §8.

## 12. ¿Cuánto pesa una fila?

- **Cómo**: después del primer envío real, `SELECT max(length(data_json))` en
  `vetter_rows`.
- **Bloquea**: el tamaño de los chunks (hoy 90 KB) y el `WITHOUT ROWID` de
  `vetter_rows`. Si las filas son de varios KB, se revisa.
