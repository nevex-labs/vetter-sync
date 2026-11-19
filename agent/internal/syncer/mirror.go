package syncer

import (
	"fmt"

	"vetter-sync/agent/internal/dbf"
)

// La clínica tiene dos copias vivas: el Vetter de esta PC escribe en
// `OneDrive\BKP Vetter5` y el agente lee el UNC. Divergen —OneDrive suele tener
// más registros, el UNC escrituras más nuevas— y un `robocopy /MIR` de otra
// máquina puede pisar lo que esta PC cargó en OneDrive.
//
// El agente no puede evitar esa pérdida, pero sí hacerla visible: compara la
// cantidad de registros de cada tabla entre las dos rutas y lo manda en
// `agent_error`. Abre solo el header; nunca lee filas de la réplica, así que no
// hay riesgo de importar datos viejos.
//
// Cantidades iguales no significan copias iguales: `VACUNAS` tiene los mismos
// registros en las dos y aun así el UNC tiene escrituras más nuevas. El silencio
// de este aviso no es un "está sincronizado".
func mirrorDifferences(source, mirror string) []string {
	var out []string

	for _, table := range Tables() {
		// `findTable` compara sin distinguir mayúsculas: en la clínica conviven
		// CLIENTES.DBF y visitas.dbf.
		fromPath, err := findTable(source, table)
		if err != nil {
			return []string{"mirror: no se pudo leer el UNC"}
		}

		toPath, err := findTable(mirror, table)
		if err != nil {
			// Si no se puede leer la réplica, una línea y se corta: repetirla
			// cinco veces con rutas y errores del sistema pasa el tope de
			// `agent_error` y tapa otras fallas.
			return []string{"mirror: no se pudo leer OneDrive"}
		}

		from, err := dbf.Records(fromPath)
		if err != nil {
			return []string{"mirror: no se pudo leer el UNC"}
		}

		to, err := dbf.Records(toPath)
		if err != nil {
			return []string{"mirror: no se pudo leer OneDrive"}
		}

		if from != to {
			out = append(out, fmt.Sprintf("mirror: %s UNC %d, OneDrive %d", table, from, to))
		}
	}

	return out
}
