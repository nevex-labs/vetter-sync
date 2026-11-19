package syncer

import (
	"fmt"
	"path/filepath"
	"strings"

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
func mirrorDifferences(source, mirror string) []string {
	var out []string

	for _, table := range Tables() {
		name := strings.ToUpper(table) + ".dbf"

		from, err := recordsOf(filepath.Join(source, name))
		if err != nil {
			out = append(out, fmt.Sprintf("%s: no se pudo leer el UNC (%v)", table, err))
			continue
		}

		to, err := recordsOf(filepath.Join(mirror, name))
		if err != nil {
			// La réplica puede no existir o estar tomada por OneDrive: se avisa
			// una vez y se sigue; no es motivo para cortar el sync.
			out = append(out, fmt.Sprintf("%s: no se pudo leer OneDrive (%v)", table, err))
			continue
		}

		if from != to {
			out = append(out, fmt.Sprintf("%s: UNC %d, OneDrive %d", table, from, to))
		}
	}

	return out
}

func recordsOf(path string) (int, error) {
	table, err := dbf.Open(path)
	if err != nil {
		return 0, fmt.Errorf("abrir %s: %w", path, err)
	}

	return table.Computed, nil
}
