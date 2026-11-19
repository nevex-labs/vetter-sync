package syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"vetter-sync/agent/internal/dbf"
)

// maxAgentError es el largo del texto que se manda en agent_error. El Worker
// lo trunca igual; acá se corta para no mandar de más.
const maxAgentError = 500

// TableReport son los conteos de una tabla en una corrida. Sin valores.
type TableReport struct {
	Table      string `json:"tabla"`
	Unchanged  bool   `json:"sin_cambios"`
	Rows       int    `json:"filas"`
	Deleted    int    `json:"borradas"`
	Pending    int    `json:"pendientes"`
	Sent       int    `json:"enviadas"`
	Confirmed  int    `json:"confirmadas"`
	Duplicates int    `json:"ids_duplicados"`
	EmptyKey   int    `json:"sin_id"`
	Missing    int    `json:"desaparecidas"`
	Error      string `json:"error,omitempty"`
}

// Summary es el resultado de una corrida.
type Summary struct {
	Tables []TableReport
	Errors []string
}

// Runner arma y manda los lotes. Client es nil en --dry-run.
type Runner struct {
	Config Config
	Client *Client
	Logger *slog.Logger
	DryRun bool
}

// Run hace una corrida completa. Devuelve error solo si hay que cortar
// (snapshot ilegible, 401); los problemas de una tabla quedan en el Summary y
// viajan en agent_error con el heartbeat.
func (r *Runner) Run(ctx context.Context) (Summary, error) {
	snap, err := LoadSnapshot(r.Config.Snapshot)
	if err != nil {
		return Summary{}, err
	}

	var summary Summary

	serverDown := false

	if !r.DryRun {
		serverDown, err = r.flushOutbox(ctx, &snap)
		if err != nil {
			return summary, r.finish(&snap, err)
		}
	}

	state := runState{budget: r.Config.MaxRowsPerRun, serverDown: serverDown}

	for _, table := range Tables() {
		report, err := r.syncTable(ctx, table, &snap, &state)
		summary.Tables = append(summary.Tables, report)

		if report.Error != "" {
			summary.Errors = append(summary.Errors, table+": "+report.Error)
		}

		if err != nil {
			return summary, r.finish(&snap, err)
		}
	}

	serverDown = state.serverDown

	// Divergencia entre el UNC y lo que escribe el Vetter de esta PC. No corta
	// la corrida: viaja en agent_error para que se vea en el panel.
	if r.Config.MirrorPath != "" {
		summary.Errors = append(summary.Errors, mirrorDifferences(r.Config.ZPath, r.Config.MirrorPath)...)
	}

	if r.DryRun {
		return summary, nil
	}

	if serverDown {
		summary.Errors = append(summary.Errors, "servidor o red caídos: pendientes en el outbox")
	} else if err := r.heartbeat(ctx, summary.Errors); err != nil {
		return summary, r.finish(&snap, err)
	}

	return summary, r.finish(&snap, nil)
}

// runState es lo que se arrastra de una tabla a la siguiente en una corrida.
type runState struct {
	budget     int
	serverDown bool
}

// syncTable lee una tabla y manda lo pendiente dentro del presupuesto de la
// corrida. La tabla se da por cerrada (size, mtime) solo si se confirmó todo.
func (r *Runner) syncTable(ctx context.Context, table string, snap *Snapshot, state *runState) (TableReport, error) {
	report, pending, stamp := r.prepare(table, snap)

	if r.DryRun || report.Error != "" || report.Unchanged {
		return report, nil
	}

	take := min(state.budget, len(pending))
	state.budget -= take

	if state.serverDown {
		take = 0
	}

	sent, down, err := r.sendRows(ctx, snap, pending[:take])
	report.Sent, report.Confirmed = take, sent
	state.serverDown = state.serverDown || down

	if err != nil {
		return report, err
	}

	if sent == len(pending) && stamp != nil {
		snap.Files[table] = *stamp
	}

	return report, nil
}

func (r *Runner) finish(snap *Snapshot, cause error) error {
	if r.DryRun {
		return cause
	}

	if err := snap.Save(r.Config.Snapshot); err != nil {
		return errors.Join(cause, err)
	}

	return cause
}

// prepare copia la tabla, la parsea y devuelve las filas cuyo hash no está
// confirmado en el snapshot. stamp es nil si la copia no fue estable.
func (r *Runner) prepare(table string, snap *Snapshot) (TableReport, []Row, *FileStamp) {
	report := TableReport{Table: table}

	key := r.Config.Keys[table]
	if key == "" {
		report.Error = "sin key en config.keys: no se adivina"
		r.Logger.Error("tabla sin key", "tabla", table)

		return report, nil, nil
	}

	src, err := findTable(r.Config.ZPath, table)
	if err != nil {
		report.Error = "DBF no encontrado"
		r.Logger.Error("tabla no encontrada", "tabla", table, "err", err)

		return report, nil, nil
	}

	if !r.DryRun {
		if stamp, err := stampOf(src); err == nil && snap.Files[table] == stamp {
			report.Unchanged = true
			r.Logger.Info("tabla sin cambios", "tabla", table)

			return report, nil, nil
		}
	}

	local, err := copyTable(src, r.Config.CopyDir, table)
	if err != nil {
		report.Error = "no se pudo copiar el DBF"
		r.Logger.Error("copia", "tabla", table, "err", err)

		return report, nil, nil
	}

	if !local.Stable {
		r.Logger.Warn("el DBF cambió durante la copia", "tabla", table)
	}

	pending, err := collect(local.Path, table, key, snap, &report)
	if err != nil {
		report.Error = "no se pudo leer la copia"
		r.Logger.Error("parser", "tabla", table, "err", err)

		return report, nil, nil
	}

	r.Logger.Info("tabla leída", "tabla", table, "filas", report.Rows, "borradas", report.Deleted,
		"pendientes", report.Pending, "ids_duplicados", report.Duplicates, "sin_id", report.EmptyKey,
		"desaparecidas", report.Missing)

	if !local.Stable {
		return report, pending, nil
	}

	return report, pending, &local.Stamp
}

// collect lee la copia y arma las filas que cambiaron. Si un vetter_id se
// repite en la tabla gana la última, igual que en el Worker, salvo que la
// última esté borrada y la anterior no: un borrado viejo no anula una fila viva.
func collect(path, table, key string, snap *Snapshot, report *TableReport) ([]Row, error) {
	seen := map[string]int{}
	rows := make([]Row, 0, 64)
	parts := strings.Split(key, "+")

	err := dbf.ReadRows(path, func(raw dbf.Row) error {
		report.Rows++

		if raw.Deleted {
			report.Deleted++
		}

		id := vetterID(raw.Values, parts)
		if id == "" {
			report.EmptyKey++
			return nil
		}

		hash, err := Hash(raw.Values, raw.Deleted)
		if err != nil {
			return err
		}

		row := Row{Tabla: table, VetterID: id, Hash: hash, Deleted: raw.Deleted, Data: raw.Values}

		if at, dup := seen[id]; dup {
			report.Duplicates++

			if !row.Deleted || rows[at].Deleted {
				rows[at] = row
			}

			return nil
		}

		seen[id] = len(rows)
		rows = append(rows, row)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("leer %s: %w", table, err)
	}

	pending := rows[:0]

	for _, row := range rows {
		if snap.Rows[rowKey(table, row.VetterID)] != row.Hash {
			pending = append(pending, row)
		}
	}

	prefix := table + "|"

	for stored := range snap.Rows {
		if id, ok := strings.CutPrefix(stored, prefix); ok {
			if _, present := seen[id]; !present {
				report.Missing++
			}
		}
	}

	report.Pending = len(pending)

	return pending, nil
}

// vetterID arma el id con los campos de la key: "1524", "1524/2" o, con una
// key compuesta "A+B", los valores unidos por "|". Si falta alguno, no hay id.
func vetterID(values map[string]any, fields []string) string {
	parts := make([]string, 0, len(fields))

	for _, field := range fields {
		part := formatID(values[strings.TrimSpace(field)])
		if part == "" {
			return ""
		}

		parts = append(parts, part)
	}

	return strings.Join(parts, "|")
}

func formatID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

// sendRows manda en lotes de 200 y confirma en el snapshot solo lo que el
// Worker respondió con 200. Devuelve cuántas filas quedaron confirmadas y si
// el servidor quedó caído (lo pendiente va al outbox).
func (r *Runner) sendRows(ctx context.Context, snap *Snapshot, rows []Row) (int, bool, error) {
	confirmed := 0

	for start := 0; start < len(rows); start += MaxBatchRows {
		batch := rows[start:min(start+MaxBatchRows, len(rows))]

		ok, err := r.sendSplitting(ctx, snap, batch)
		confirmed += ok

		switch {
		case err == nil:
		case errors.Is(err, ErrUnauthorized):
			r.Logger.Error("401: se corta la corrida, revisar el pareo")
			return confirmed, false, err
		case errors.Is(err, ErrServer):
			r.Logger.Error("servidor o red caídos tras reintentos", "err", err)

			if err := r.toOutbox(rows[start:]); err != nil {
				return confirmed, true, err
			}

			return confirmed, true, nil
		default:
			return confirmed, false, err
		}
	}

	return confirmed, false, nil
}

// sendSplitting manda un lote y, si es demasiado grande (413), lo parte a la
// mitad. Un 400 se loguea y se sigue: es un bug del agente.
func (r *Runner) sendSplitting(ctx context.Context, snap *Snapshot, batch []Row) (int, error) {
	if len(batch) == 0 {
		return 0, nil
	}

	resp, err := r.Client.Send(ctx, Batch{Rows: batch})

	switch {
	case err == nil:
		for _, row := range batch {
			snap.Rows[rowKey(row.Tabla, row.VetterID)] = row.Hash
		}

		r.Logger.Info("lote enviado", "tabla", batch[0].Tabla, "filas", len(batch),
			"changed", resp.Changed, "unchanged", resp.Unchanged, "rejected", resp.Rejected)

		if err := snap.Save(r.Config.Snapshot); err != nil {
			return len(batch), err
		}

		return len(batch), nil
	case errors.Is(err, ErrTooLarge) && len(batch) > 1:
		half := len(batch) / 2
		r.Logger.Warn("413: se parte el lote", "tabla", batch[0].Tabla, "filas", len(batch))

		first, err := r.sendSplitting(ctx, snap, batch[:half])
		if err != nil {
			return first, err
		}

		second, err := r.sendSplitting(ctx, snap, batch[half:])

		return first + second, err
	case errors.Is(err, ErrTooLarge):
		r.Logger.Error("413 con una sola fila: no entra", "tabla", batch[0].Tabla, "vetter_id", batch[0].VetterID)
		return 0, nil
	case errors.Is(err, ErrBadRequest):
		r.Logger.Error("400: lote mal armado, se sigue", "tabla", batch[0].Tabla, "filas", len(batch))
		return 0, nil
	default:
		return 0, err
	}
}

func (r *Runner) heartbeat(ctx context.Context, problems []string) error {
	batch := Batch{AgentError: agentError(problems)}

	resp, err := r.Client.Send(ctx, batch)
	if err != nil {
		r.Logger.Error("heartbeat", "err", err)

		if errors.Is(err, ErrUnauthorized) {
			return err
		}

		return nil
	}

	r.Logger.Info("heartbeat", "received", resp.Received, "agent_error", batch.AgentError != "")

	return nil
}

// agentError arma el texto corto del panel. Solo nombres de tabla y causas
// genéricas: nunca valores.
func agentError(problems []string) string {
	text := strings.Join(problems, "; ")

	if len(text) > maxAgentError {
		text = strings.ToValidUTF8(text[:maxAgentError], "")
	}

	return text
}

// ReportFailure manda {"rows":[],"agent_error":...} cuando la corrida no pudo
// ni empezar (share caído, contrato distinto). Es el mejor esfuerzo: si
// tampoco hay red, queda en el log.
func ReportFailure(ctx context.Context, client *Client, logger *slog.Logger, problem string) {
	if client == nil || client.Secret == "" {
		return
	}

	// Sin reintentos: si la corrida ya falló, no se queda esperando.
	once := *client
	once.Backoff = nil

	if _, err := once.Send(ctx, Batch{AgentError: agentError([]string{problem})}); err != nil {
		logger.Error("no se pudo reportar agent_error", "err", err)
	}
}
