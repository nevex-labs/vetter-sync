package syncer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientSend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		codes     []int
		backoff   int
		wantErr   error
		wantCalls int32
	}{
		{name: "200", codes: []int{200}, wantCalls: 1},
		{name: "401 no reintenta", codes: []int{401}, backoff: 3, wantErr: ErrUnauthorized, wantCalls: 1},
		{name: "400 no reintenta", codes: []int{400}, backoff: 3, wantErr: ErrBadRequest, wantCalls: 1},
		{name: "413 no reintenta", codes: []int{413}, backoff: 3, wantErr: ErrTooLarge, wantCalls: 1},
		{name: "500 reintenta y entra", codes: []int{500, 502, 200}, backoff: 3, wantCalls: 3},
		{name: "500 agota reintentos", codes: []int{500, 500, 500}, backoff: 2, wantErr: ErrServer, wantCalls: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer vgs_test" || r.Header.Get("Content-Type") != "application/json" {
					w.WriteHeader(http.StatusTeapot)
					return
				}

				n := calls.Add(1)
				code := tt.codes[min(int(n), len(tt.codes))-1]
				w.WriteHeader(code)

				if code == http.StatusOK {
					_, _ = w.Write([]byte(`{"received":0,"unchanged":0,"changed":0,"duplicated":0,"applied":0,"ambiguous":0,"rejected":0}`))
				}
			}))
			t.Cleanup(server.Close)

			client := &Client{
				Endpoint: server.URL,
				Secret:   "vgs_test",
				HTTP:     server.Client(),
				Backoff:  make([]time.Duration, tt.backoff),
			}

			_, err := client.Send(context.Background(), Batch{})
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, quería %v", err, tt.wantErr)
			}

			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("llamadas = %d, quería %d", got, tt.wantCalls)
			}
		})
	}
}

func TestClientPing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		_, _ = w.Write([]byte(`{"ok":true,"contract":3}`))
	}))
	t.Cleanup(server.Close)

	client := &Client{Endpoint: server.URL, HTTP: server.Client()}

	contract, err := client.Ping(context.Background())
	if err != nil || contract != Contract {
		t.Errorf("contract = %d, err = %v", contract, err)
	}
}

func TestSendMandaLaVersion(t *testing.T) {
	t.Parallel()

	var body string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"received":0}`))
	}))
	t.Cleanup(server.Close)

	client := &Client{
		Endpoint:  server.URL,
		Secret:    "vgs_test",
		UserAgent: "vetter-agent/test",
		Version:   "0.3.3",
		HTTP:      server.Client(),
	}

	if _, err := client.Send(context.Background(), Batch{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(body, `"agent_version":"0.3.3"`) {
		t.Errorf("el lote no lleva la versión: %s", body)
	}
}

func TestEncodeBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		batch Batch
		want  string
	}{
		{name: "heartbeat", batch: Batch{}, want: `{"rows":[]}`},
		{name: "con error", batch: Batch{AgentError: "share inalcanzable"}, want: `{"rows":[],"agent_error":"share inalcanzable"}`},
		{
			name:  "fila",
			batch: Batch{Rows: []Row{{Tabla: "CLIENTES", VetterID: "1", Hash: "h", Deleted: true, Data: map[string]any{"A": nil}}}},
			want:  `{"rows":[{"tabla":"CLIENTES","vetter_id":"1","hash":"h","deleted":true,"data":{"A":null}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodeBatch(tt.batch)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Errorf("body = %s\nquería %s", got, tt.want)
			}
		})
	}
}

func TestAgentErrorTruncatesOnRune(t *testing.T) {
	t.Parallel()

	got := agentError([]string{strings.Repeat("ñ", maxAgentError)})

	if len(got) > maxAgentError || !strings.HasPrefix(got, "ññ") || strings.ContainsRune(got, '�') {
		t.Errorf("agent_error mal cortado: %d bytes", len(got))
	}
}
