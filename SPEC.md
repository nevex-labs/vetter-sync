# Spec: Agente Vetter read-only + ingesta Worker

## 1. Objetivo

Traer los datos que hoy solo están en Vetter 5 a nuestro sistema, sin pagar el módulo de Vetter y sin dejar de usar Vetter.

- Vetter sigue siendo el sistema de carga (vete carga ahí como siempre).
- Nuestro sistema (Worker + D1 + R2) es solo lectura de una copia, para el bot de WhatsApp y recordatorios.

## 2. No-objetivos

- No escribir nunca en DBF/FPT de Vetter.
- No reemplazar Vetter, no turnos bidireccionales.
- No API local en la PC, no abrir puertos, no túnel.
- No conversación con Vetter Sistemas en esta fase.

## 3. Arquitectura

```
PC vete (Windows)                    Cloudflare
Z:\vetter5\Datos (prod)              Worker veterinariagachens
  │ copia RO → %TEMP%                  │
  ▼                                    ▼
vetter-agent.exe (Go, tarea) ──HTTPS POST──► POST /api/sync/vetter
  snapshot.db local                      │ valida token, upsert D1, sube R2
                                         ▼
                                    D1 + R2 (bot/panel leen de acá)
```

Por qué así y no API local:

- Worker no puede iniciar conexión hacia una PC detrás de NAT. El agente empuja hacia afuera, como un navegador.
- Era lo que encarecía el presupuesto Vetter (API + bridge + red privada). Acá hay una sola pieza afuera: un endpoint de ingesta.

## 4. Qué se extrae (mapeo)

Fuente: diagnóstico remoto del 15/10 (hipótesis a confirmar con la copia). Destino: D1 actual.

| Vetter (hipótesis) | Nuestro D1 | Notas |
|---|---|---|
| `CLIENTES.CODIGO, NOMBRE, TELEFONO(40c texto libre)` | `tutors` + `tutor_phones` | Un tutor por cliente y un teléfono por cada número válido. No toca `conversations` ni `contact_name` (ADR-0013). |
| `PACIENTES.CODIGOPACI texto tipo 1524/2` | `pets.vetter_id UNIQUE` | Llave estable. Hipótesis a confirmar: el prefijo es el código de cliente. |
| `PACIENTES nombre, especie, raza, sexo` | `pets.nombre, nombre_norm, especie, raza` | `nombre_norm = lower(trim)`. Sin UNIQUE por homónimos. Sexo y nacimiento esperan el schema confirmado. |
| `VACUNAS.FECHA_REVA` próxima dosis | `preventives.proxima_dosis` | `AVISADO` no se usa. Importar no avisa: el recordatorio del ADR-0015 no está implementado. |
| `VISITAS` bitácora con fecha/hora | `visits` + `visit_notes` | Única tabla con timestamp por evento. Las notas son append-only; la deduplicación va en el paso 4. |
| `ADJUNTOS` + PDFs/JPGs | `attachments.r2_key` + R2 `veterinaria-gachen-files` | Layout `vetter/<tabla>/<vetter_id>/<nombre>`. Reimportar no duplica (`r2_key` UNIQUE). |
| Turnos Vetter | nada (0 registros según reporte) | Nuestro bot es el sistema de turnos, sin conflicto. |

Tablas clínicas nuevas (`preventives`, `attachments`) referencian `pets.id`, nunca strings (ADR-004).

**El agente no mapea.** Manda las filas crudas de la lista blanca con su hash y el mapeo vive en el Worker (ADR-0016). Así se puede rehacer sin volver a la PC.

## 5. Cómo detecta cambios (sin fecha de modificación)

Reporte dice: ningún campo guarda fecha de modificación. Estrategia combinada:

1. `VISITAS` como bitácora (qué paciente se tocó, cuándo).
2. IDs autoincrementales (lo nuevo).
3. `mtime` de DBF/FPT para decidir si vale copiar.
4. Hash por fila vs `snapshot.db` local (SQLite del agente) para ediciones sin fecha.

Limitación: ediciones sin cambio de fecha solo se ven por hash. Borrados lógicos (`deleted flag`) a confirmar en copia.

## 6. Agente Go (spec)

- Un `.exe` único, sin runtime. Corre como Tarea Programada cada 5 min.
- Config: `Z_PATH, COPY_DIR, SNAPSHOT_DB, ENDPOINT, TOKEN, INSTALL_ID`. Token en archivo con ACL solo al usuario servicio.
- **`Z_PATH` va por UNC** (`\\Srvarchvete01\c\vetter5\Datos`), no por letra de unidad: una tarea que corre como SYSTEM o "sin que el usuario inicie sesión" no ve `Z:`, que es por sesión.
- Flujo: mira `mtime` → copia con share de solo lectura → parsea DBF → arma el hash de cada fila → diff vs snapshot → `POST /api/sync/vetter` con lotes de hasta 200 filas → reintenta con backoff, acumula offline. No mapea ni manda PII en los logs.
- Nunca lee de `C:\Users\...\OneDrive\BKP Vetter5` (réplica divergente). Siempre `Z:` o su UNC.
- Logs locales rotados, sin PII (solo conteos + `vetter_id`).

## 7. Endpoint Worker `POST /api/sync/vetter`

- Vive en el Worker `veterinariagachens`, no en uno aparte (ADR-0016): D1 y R2 están ahí.
- Auth: `Authorization: Bearer <token por instalación>` (`VETTER_SYNC_TOKEN`) comparado con `safeEqual`. Sin secret configurado responde 401.
- Cuerpo: `{ install_id, rows: [{ tabla, vetter_id, hash, deleted, data }] }`, hasta 200 filas y 1 MB.
- Guarda en `vetter_rows` (zona de llegada) y **no proyecta** en el paso 1. Respuesta `{received, unchanged, changed, duplicated, applied, ambiguous, rejected}`; `applied` y `ambiguous` quedan en 0 hasta el paso 3.
- Idempotente: si el hash no cambió, no escribe ni mueve `received_at`. Nunca borra.
- Lista blanca: `CLIENTES`, `PACIENTES`, `VACUNAS`, `VISITAS`, `ADJUNTOS`. El resto se rechaza y se cuenta.
- PDFs: el agente sube directo a R2 con URL firmada o manda bytes en lote chico. Layout: `vetter/<tabla>/<vetter_id>/<nombre>`.
- Volcado inicial 60k registros: repartir en días, lotes chicos. Free tiene 50 subrequests/invocación. Si se pasa, subir a pago o bajar `FILES_PER_RUN`.

## 8. Operación y riesgos

- Previo obligatorio: ok escrito del vete (WhatsApp/mail vale) por Ley 25.326. Sin esto no se trae 1 solo tutor.
- Crítico ya: avisarle que trabaja sobre OneDrive y no sobre `Z:`. Próximo robocopy `/MIR` puede pisar datos del 14/10. Independiente de nosotros.
- Licencia Vetter: lectura de copia no deja rastro en Vetter. Riesgo bajo. Escritura = corrupción + baja de licencia. Prohibida.
- Duda abierta: destino MySQL ofuscado en `SQLTEST.DBF` (¿chequeo licencia?). Falta observar arranque con monitor de red.
- Costo: lotes cada 5 min entran en free. El ZIP 458MB y 60k filas iniciales son el único pico a cuidar en R2/D1.

## 9. Fases

1. **Frenar daño**: confirmar `Z:` vs OneDrive, pedir ok escrito. 1 día.
2. **Cerrar telemetría**: arranque con `Get-NetTCPConnection` corriendo. 1 hora en la vete.
3. **Prototipo lector**: Go lee copia, imprime conteos sin PII. Valida `CODIGOPACI`, teléfonos, `FECHA_REVA`.
4. **ADR + endpoint + `vacunas/estudios` migrations**.
5. **Piloto**: 1 semana solo diff, sin PDFs. Después PDFs chicos, resto no.
