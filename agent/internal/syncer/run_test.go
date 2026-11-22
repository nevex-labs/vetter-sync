package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"vetter-sync/agent/internal/dbftest"
)

// fakeWorker graba los lotes y responde según status.
type fakeWorker struct {
	mu      sync.Mutex
	batches []Batch
	status  func(Batch) int
}

func (f *fakeWorker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var batch Batch

	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	f.batches = append(f.batches, batch)
	code := http.StatusOK

	if f.status != nil {
		code = f.status(batch)
	}
	f.mu.Unlock()

	w.WriteHeader(code)

	if code == http.StatusOK {
		_ = json.NewEncoder(w).Encode(Response{Received: len(batch.Rows), Changed: len(batch.Rows)})
	}
}

func (f *fakeWorker) take() []Batch {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := f.batches
	f.batches = nil

	return out
}

func rowsIn(batches []Batch) []Row {
	var out []Row

	for _, batch := range batches {
		out = append(out, batch.Rows...)
	}

	return out
}

type fixture struct {
	runner Runner
	worker *fakeWorker
	dir    string
}

// newFixture arma dos tablas sintéticas: CLIENTES (con un borrado) y
// visitas.dbf en minúscula, como en la clínica. VACUNAS, ADJUNTOS y
// PACIENTES quedan sin key para probar que no se adivina.
func newFixture(t *testing.T) *fixture {
	t.Helper()

	root := t.TempDir()
	share := filepath.Join(root, "share")

	if err := os.MkdirAll(share, 0o750); err != nil {
		t.Fatal(err)
	}

	dbftest.Write(t, share, "CLIENTES.DBF", 0x03,
		[]dbftest.Field{{Name: "CODIGO", Type: 'N', Length: 6}, {Name: "NOMBRE", Type: 'C', Length: 10}},
		[]dbftest.Record{
			{Values: []string{"  1524", "Ana"}},
			{Deleted: true, Values: []string{"  1525", "Beto"}},
			{Values: []string{"      ", "Sin codigo"}},
		})

	dbftest.Write(t, share, "visitas.dbf", 0x03,
		[]dbftest.Field{
			{Name: "FECHA", Type: 'T', Length: 8},
			{Name: "CODIGO", Type: 'N', Length: 10},
			{Name: "DETALLE", Type: 'C', Length: 20},
		},
		[]dbftest.Record{
			{Values: []string{dbftest.DateTime(time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)), "        77", "control"}},
		})

	worker := &fakeWorker{}
	server := httptest.NewServer(worker)
	t.Cleanup(server.Close)

	cfg := Config{
		ZPath:         share,
		CopyDir:       filepath.Join(root, "copy"),
		Snapshot:      filepath.Join(root, "snapshot.json"),
		Endpoint:      server.URL,
		InstallID:     "pc-test",
		MaxRowsPerRun: 5000,
		Keys:          map[string]string{"CLIENTES": "CODIGO", "VISITAS": "CODIGO"},
	}

	return &fixture{
		runner: Runner{
			Config: cfg,
			Client: &Client{Endpoint: server.URL, Secret: "vgs_test", HTTP: server.Client()},
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		worker: worker,
		dir:    share,
	}
}

func TestRunTwiceSecondSendsNothing(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	ctx := context.Background()

	if _, err := fx.runner.Run(ctx); err != nil {
		t.Fatal(err)
	}

	first := fx.worker.take()
	sent := rowsIn(first)

	if len(sent) != 3 {
		t.Fatalf("primera corrida: %d filas, quería 3 (2 CLIENTES con id + 1 VISITAS)", len(sent))
	}

	byID := map[string]Row{}
	for _, row := range sent {
		byID[row.Tabla+"|"+row.VetterID] = row
	}

	deleted, ok := byID["CLIENTES|1525"]
	if !ok || !deleted.Deleted {
		t.Errorf("el borrado tiene que viajar con deleted=true y vetter_id de keys: %+v", deleted)
	}

	if deleted.RowKey != "1525" {
		t.Errorf("row_key = %q, quería el vetter_id", deleted.RowKey)
	}

	if _, ok := byID["VISITAS|77"]; !ok {
		t.Errorf("visitas.dbf en minúscula no se encontró: %v", byID)
	}

	heartbeat := first[len(first)-1]
	if len(heartbeat.Rows) != 0 || !strings.Contains(heartbeat.AgentError, "VACUNAS") {
		t.Errorf("el último lote es el heartbeat con agent_error de las tablas sin key: %+v", heartbeat)
	}

	if _, err := fx.runner.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if rows := rowsIn(fx.worker.take()); len(rows) != 0 {
		t.Errorf("segunda corrida: %d filas, quería 0", len(rows))
	}
}

// Cambia el mtime pero no el contenido: se relee y no manda nada.
func TestRunSameContentNewMtime(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	ctx := context.Background()

	if _, err := fx.runner.Run(ctx); err != nil {
		t.Fatal(err)
	}

	fx.worker.take()

	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(fx.dir, "CLIENTES.DBF"), future, future); err != nil {
		t.Fatal(err)
	}

	summary, err := fx.runner.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if rows := rowsIn(fx.worker.take()); len(rows) != 0 {
		t.Errorf("mismo contenido con otro mtime: %d filas, quería 0", len(rows))
	}

	if summary.Tables[0].Unchanged || summary.Tables[0].Rows != 3 {
		t.Errorf("CLIENTES tenía que releerse: %+v", summary.Tables[0])
	}
}

func TestRunStatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    func(Batch) int
		wantErr   error
		wantPosts int
		wantSnap  int
	}{
		{
			name: "413 parte el lote",
			status: func(b Batch) int {
				if len(b.Rows) > 1 {
					return http.StatusRequestEntityTooLarge
				}

				return http.StatusOK
			},
			// CLIENTES: 1 de 2 + 2 de 1; VISITAS: 1; heartbeat: 1.
			wantPosts: 5,
			wantSnap:  3,
		},
		{
			name:      "401 corta la corrida",
			status:    func(Batch) int { return http.StatusUnauthorized },
			wantErr:   ErrUnauthorized,
			wantPosts: 1,
			wantSnap:  0,
		},
		{
			name: "400 se loguea y se sigue",
			status: func(b Batch) int {
				if len(b.Rows) > 0 && b.Rows[0].Tabla == "CLIENTES" {
					return http.StatusBadRequest
				}

				return http.StatusOK
			},
			wantPosts: 3,
			wantSnap:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newFixture(t)
			fx.worker.status = tt.status

			_, err := fx.runner.Run(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, quería %v", err, tt.wantErr)
			}

			if posts := len(fx.worker.take()); posts != tt.wantPosts {
				t.Errorf("POSTs = %d, quería %d", posts, tt.wantPosts)
			}

			snap, err := LoadSnapshot(fx.runner.Config.Snapshot)
			if err != nil {
				t.Fatal(err)
			}

			if len(snap.Rows) != tt.wantSnap {
				t.Errorf("confirmadas en snapshot = %d, quería %d", len(snap.Rows), tt.wantSnap)
			}
		})
	}
}

func TestRunOutboxOnServerError(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	fx.worker.status = func(Batch) int { return http.StatusInternalServerError }
	ctx := context.Background()

	summary, err := fx.runner.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if got := OutboxSize(fx.runner.Config.CopyDir); got == 0 {
		t.Fatal("con 500 lo pendiente tiene que quedar en el outbox")
	}

	if !strings.Contains(strings.Join(summary.Errors, ";"), "outbox") {
		t.Errorf("errores = %v", summary.Errors)
	}

	fx.worker.take()
	fx.worker.status = nil

	if _, err := fx.runner.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if got := OutboxSize(fx.runner.Config.CopyDir); got != 0 {
		t.Errorf("outbox = %d, quería vacío", got)
	}

	if rows := rowsIn(fx.worker.take()); len(rows) != 3 {
		t.Errorf("filas reenviadas = %d, quería 3 (sin duplicar)", len(rows))
	}
}

func TestRunMaxRowsPerRun(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	fx.runner.Config.MaxRowsPerRun = 1
	ctx := context.Background()

	for run, want := range []int{1, 1, 1, 0} {
		if _, err := fx.runner.Run(ctx); err != nil {
			t.Fatal(err)
		}

		if got := len(rowsIn(fx.worker.take())); got != want {
			t.Errorf("corrida %d: %d filas, quería %d", run+1, got, want)
		}
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	fx.runner.DryRun = true
	fx.runner.Client = nil

	summary, err := fx.runner.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if posts := len(fx.worker.take()); posts != 0 {
		t.Errorf("dry-run hizo %d POSTs", posts)
	}

	if _, err := os.Stat(fx.runner.Config.Snapshot); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry-run no puede escribir el snapshot: %v", err)
	}

	clientes := summary.Tables[0]
	if clientes.Rows != 3 || clientes.Deleted != 1 || clientes.Pending != 2 || clientes.EmptyKey != 1 {
		t.Errorf("CLIENTES = %+v", clientes)
	}

	if len(summary.Errors) != 3 {
		t.Errorf("tres tablas sin key, errores = %v", summary.Errors)
	}
}

func TestVetterID(t *testing.T) {
	t.Parallel()

	values := map[string]any{"CODIGO": int64(1524), "CODIGOPACI": "1524/2", "FECHA": "2026-10-14T09:00:00", "VACIO": nil}

	tests := []struct {
		key  string
		want string
	}{
		{key: "CODIGO", want: "1524"},
		{key: "CODIGOPACI", want: "1524/2"},
		{key: "CODIGO+FECHA", want: "1524|2026-10-14T09:00:00"},
		{key: "CODIGO+VACIO", want: ""},
		{key: "NO_EXISTE", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			if got := vetterID(values, strings.Split(tt.key, "+")); got != tt.want {
				t.Errorf("vetterID(%s) = %q, quería %q", tt.key, got, tt.want)
			}
		})
	}
}

// Un borrado viejo con el mismo id no anula la fila viva.
func TestCollectDeletedDuplicateKeepsLive(t *testing.T) {
	t.Parallel()

	path := dbftest.Write(t, t.TempDir(), "PACIENTES.DBF", 0x03,
		[]dbftest.Field{{Name: "CODIGOPACI", Type: 'C', Length: 9}},
		[]dbftest.Record{
			{Values: []string{"1524/2"}},
			{Deleted: true, Values: []string{"1524/2"}},
		})

	snap := Snapshot{Rows: map[string]string{}, Files: map[string]FileStamp{}}
	report := TableReport{}

	rows, err := collect(path, "PACIENTES", "CODIGOPACI", &snap, &report)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Fatalf("filas = %d, quería las dos", len(rows))
	}

	// La viva se queda la clave base: es la que se proyecta.
	if rows[0].Deleted || rows[0].RowKey != "1524/2" {
		t.Errorf("primera fila = %+v", rows[0])
	}

	if !rows[1].Deleted || rows[1].RowKey != "1524/2#2" {
		t.Errorf("segunda fila = %+v", rows[1])
	}
}

// Dos vivas con el mismo id: la clave base se queda con la última, igual que la
// 0.1.1, para que la primera corrida de 0.2.x no cambie el contenido de la base.
func TestCollectLiveDuplicateKeepsLastInBase(t *testing.T) {
	t.Parallel()

	path := dbftest.Write(t, t.TempDir(), "PACIENTES.DBF", 0x03,
		[]dbftest.Field{{Name: "CODIGOPACI", Type: 'C', Length: 9}, {Name: "NOMBRE", Type: 'C', Length: 8}},
		[]dbftest.Record{
			{Values: []string{"1524/2", "primera"}},
			{Values: []string{"1524/2", "segunda"}},
		})

	snap := Snapshot{Rows: map[string]string{}, Files: map[string]FileStamp{}}
	report := TableReport{}

	rows, err := collect(path, "PACIENTES", "CODIGOPACI", &snap, &report)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Fatalf("filas = %d", len(rows))
	}

	if rows[1].RowKey != "1524/2" || rows[1].Data["NOMBRE"] != "segunda" {
		t.Errorf("la base tiene que ser la última viva: %+v", rows[1])
	}

	if rows[0].RowKey != "1524/2#2" || rows[0].Data["NOMBRE"] != "primera" {
		t.Errorf("la repetida = %+v", rows[0])
	}
}

func TestCollectDuplicatesKeepBoth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := dbftest.Write(t, dir, "VACUNAS.DBF", 0x03,
		[]dbftest.Field{{Name: "ID_VACUNA", Type: 'N', Length: 4}, {Name: "VACUNA", Type: 'C', Length: 8}},
		[]dbftest.Record{
			{Values: []string{"   1", "vieja"}},
			{Values: []string{"   1", "nueva"}},
			{Values: []string{"   2", "otra"}},
		})

	snap := Snapshot{Rows: map[string]string{"VACUNAS|3": "x"}, Files: map[string]FileStamp{}}
	report := TableReport{Table: "VACUNAS"}

	rows, err := collect(path, "VACUNAS", "ID_VACUNA", &snap, &report)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 3 {
		t.Fatalf("filas = %d, quería 3", len(rows))
	}

	if rows[0].RowKey != "1#2" || rows[1].RowKey != "1" || rows[2].RowKey != "2" {
		t.Errorf("claves = %q, %q, %q", rows[0].RowKey, rows[1].RowKey, rows[2].RowKey)
	}

	// La base es la última viva: es la que ya estaba en D1 con la 0.1.1.
	if rows[1].Data["VACUNA"] != "nueva" {
		t.Errorf("la base = %+v", rows[1])
	}

	if report.Duplicates != 1 || report.Missing != 1 {
		t.Errorf("reporte = %+v", report)
	}
}
