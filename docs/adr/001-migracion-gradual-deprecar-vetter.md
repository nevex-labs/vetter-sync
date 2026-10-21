# ADR-001: Migración gradual para deprecar Vetter (fase futura)

- Estado: Propuesto
- Fecha: 2026-10-21
- Relacionado: `SPEC.md` §2 (no-objetivos), §9 (fases)

## Contexto

El spec actual trae una copia read-only de Vetter 5 a Worker + D1/R2 para el bot de WhatsApp y recordatorios. "No reemplazar Vetter" figura como no-objetivo (§2).

Lo que aprendimos armando el spec:

- Los datos de Vetter son DBF/FPT (dBase/FoxPro). Formato abierto, se lee sin Vetter.
- El agente ya copia RO y parsea. Un dump completo es leer todas las tablas en vez de solo las del §4, más los archivos de `ADJUNTOS`.
- Extraer es la parte fácil. Lo caro es el esquema sin documentar, la lógica de negocio que vive en Vetter (facturación, stock, caja, reportes, según qué use la vete) y la calidad de datos (teléfonos texto libre, encoding CP850/1252, borrados lógicos, memos FPT).

## Decisión

Deprecar Vetter queda como **fase futura**, fuera del alcance de las fases 1-5 del spec. Si se encara, se hace por reemplazo gradual (strangler), nunca big-bang:

1. El agente de sync sigue igual. Es la primera etapa del pipeline de migración, no se tira.
2. El sistema nuevo crece de a un módulo sobre los datos sincronizados: turnos (ya es nuestro), recordatorios de vacunas, ficha de paciente / historia clínica.
3. Cuando un módulo nuevo cubre el flujo completo, la vete deja de cargarlo en Vetter. Ese módulo pasa a ser fuente de verdad en D1.
4. Corte final: dump completo de todas las tablas, validación de conteos contra Vetter, Vetter queda como archivo read-only un tiempo.

Mientras dure la transición sigue vigente todo lo del spec: nunca escribir en DBF/FPT.

## Prerrequisitos antes de arrancar

- Ok escrito del vete (Ley 25.326), ya exigido en §8, ampliado a todos los datos y no solo los del bot.
- Relevar qué módulos de Vetter usa la vete de verdad. Define el tamaño del proyecto. Facturación electrónica AFIP sola es un proyecto aparte.
- Revisar EULA de Vetter por cláusulas de ingeniería inversa.
- Cerrar la duda de `SQLTEST.DBF` / MySQL ofuscado (§8).
- Esquema de Vetter confirmado contra la copia (sale de la fase 3).

## Consecuencias

- El mapeo y el parser del agente se escriben pensando en reusarlos para el dump completo: tablas genéricas, no hardcodear solo las del §4.
- Cada módulo migrado cambia la dirección de la verdad: D1 deja de ser copia para esa entidad. El sync debe dejar de pisarla (ver "Nunca borra" / upsert del §7).
- Riesgo principal es de adopción, no técnico: si el sistema nuevo no cubre el flujo diario del vete, sigue cargando en Vetter.
- No se compromete fecha ni presupuesto. Se reevalúa al terminar la fase 5 (piloto).
