package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Contract es la versión del contrato que entiende este binario. Si el GET
// devuelve otra, no se manda nada.
const Contract = 3

// MaxBodyBytes y MaxBatchRows son los límites del contrato.
const (
	MaxBodyBytes = 1_000_000
	MaxBatchRows = 200
)

// Errores del contrato que cambian lo que hace la corrida.
var (
	ErrUnauthorized = errors.New("401: secreto inválido o revocado")
	ErrBadRequest   = errors.New("400: lote mal armado")
	ErrTooLarge     = errors.New("413: lote demasiado grande")
	ErrServer       = errors.New("error del servidor o de red")
)

// Row es una fila del lote tal como la define el contrato.
type Row struct {
	Tabla    string         `json:"tabla"`
	RowKey   string         `json:"row_key,omitempty"`
	VetterID string         `json:"vetter_id"`
	Hash     string         `json:"hash"`
	Deleted  bool           `json:"deleted"`
	Data     map[string]any `json:"data"`
}

// Batch es el body del POST. Un lote sin filas es el heartbeat.
type Batch struct {
	Rows         []Row  `json:"rows"`
	AgentError   string `json:"agent_error,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`
}

// Response son los conteos que devuelve el Worker.
type Response struct {
	Received   int `json:"received"`
	Unchanged  int `json:"unchanged"`
	Changed    int `json:"changed"`
	Duplicated int `json:"duplicated"`
	Applied    int `json:"applied"`
	Ambiguous  int `json:"ambiguous"`
	Rejected   int `json:"rejected"`
}

// Client habla con /api/sync/vetter.
type Client struct {
	Endpoint  string
	Secret    string
	UserAgent string
	// Version va en cada lote: el panel muestra qué versión corre cada PC.
	Version string
	HTTP    *http.Client
	// Backoff son las esperas entre reintentos ante 500 o error de red. Su
	// largo es la cantidad de reintentos.
	Backoff []time.Duration
}

// DefaultBackoff duplica desde 1 s: 1, 2, 4 … 128 s (≈4 min en total), para
// que una corrida entre en el límite de 10 minutos de la tarea.
func DefaultBackoff() []time.Duration {
	out := make([]time.Duration, 0, 8)

	for wait := time.Second; wait <= 128*time.Second; wait *= 2 {
		out = append(out, wait)
	}

	return out
}

// Ping hace el GET público y devuelve la versión del contrato.
func (c *Client) Ping(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("armar GET: %w", err)
	}

	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GET %s: %w", c.Endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET %s: código %d", c.Endpoint, resp.StatusCode)
	}

	var body struct {
		OK       bool `json:"ok"`
		Contract int  `json:"contract"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&body); err != nil {
		return 0, fmt.Errorf("GET %s: respuesta: %w", c.Endpoint, err)
	}

	if !body.OK {
		return body.Contract, fmt.Errorf("GET %s: ok=false", c.Endpoint)
	}

	return body.Contract, nil
}

// EncodeBatch serializa el lote; nunca manda "rows": null.
func EncodeBatch(batch Batch) ([]byte, error) {
	if batch.Rows == nil {
		batch.Rows = []Row{}
	}

	data, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("serializar lote: %w", err)
	}

	return data, nil
}

// Send manda un lote. Reintenta con backoff ante 500 o error de red; 400, 401
// y 413 vuelven enseguida porque reintentar no los arregla.
func (c *Client) Send(ctx context.Context, batch Batch) (Response, error) {
	if batch.AgentVersion == "" {
		batch.AgentVersion = c.Version
	}

	body, err := EncodeBatch(batch)
	if err != nil {
		return Response{}, err
	}

	if len(body) > MaxBodyBytes {
		return Response{}, fmt.Errorf("%d bytes: %w", len(body), ErrTooLarge)
	}

	for attempt := 0; ; attempt++ {
		resp, err := c.post(ctx, body)
		if err == nil || !errors.Is(err, ErrServer) || attempt >= len(c.Backoff) {
			return resp, err
		}

		timer := time.NewTimer(c.Backoff[attempt])

		select {
		case <-ctx.Done():
			timer.Stop()
			return Response{}, fmt.Errorf("esperando reintento: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func (c *Client) post(ctx context.Context, body []byte) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("armar POST: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Response{}, fmt.Errorf("POST: %w", ctx.Err())
		}

		return Response{}, fmt.Errorf("POST: %w: %w", ErrServer, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		var out Response

		if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&out); err != nil {
			return Response{}, fmt.Errorf("POST: respuesta: %w", err)
		}

		return out, nil
	case http.StatusUnauthorized:
		return Response{}, ErrUnauthorized
	case http.StatusBadRequest:
		return Response{}, ErrBadRequest
	case http.StatusRequestEntityTooLarge:
		return Response{}, ErrTooLarge
	default:
		return Response{}, fmt.Errorf("código %d: %w", resp.StatusCode, ErrServer)
	}
}
