package syncer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
)

// Hash aplica la regla v1 del contrato: los campos más {"deleted": 0|1},
// claves ordenadas, sin espacios ni escape HTML, sha256 en hex minúscula.
//
// Cualquier cambio acá es un cambio de contrato: obliga a un volcado completo.
func Hash(data map[string]any, deleted bool) (string, error) {
	payload := make(map[string]any, len(data)+1)
	maps.Copy(payload, data)

	payload["deleted"] = 0
	if deleted {
		payload["deleted"] = 1
	}

	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(payload); err != nil {
		return "", fmt.Errorf("serializar fila: %w", err)
	}

	sum := sha256.Sum256(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))

	return hex.EncodeToString(sum[:]), nil
}
