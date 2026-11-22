// vetter-agent: lee la copia de Vetter sin escribirla nunca.
//
// Uso:
//
//	vetter-agent inspect --dir \\Srvarchvete01\c\vetter5\Datos --out inspector.json
//	vetter-agent inspect --dir ... --stat CLIENTES.TELEFONO --stat PACIENTES.CODIGOPACI
//	vetter-agent pair [--config C:\ProgramData\vetter-agent\config.json]
//	vetter-agent sync --config C:\ProgramData\vetter-agent\config.json [--dry-run]
//	vetter-agent version
//
// `inspect` es la fase 3 del SPEC: estructura, conteos y agregados sin PII.
// `pair` guarda el secreto de instalación; `sync` es la corrida de la tarea
// programada (docs/agent-implementation.md).
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"vetter-sync/agent/internal/report"
)

// Se pisan en el release con -ldflags.
var version = "0.2.0-dev"

//nolint:unused // la pisa el release con -ldflags
var updatePublicKey = ""

// Códigos de salida que guarda Task Scheduler.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(exitUsage)
	}

	switch os.Args[1] {
	case "inspect":
		inspect(os.Args[2:])
	case "version":
		fmt.Printf("vetter-agent %s\n", version)
	case "pair":
		os.Exit(pair(os.Args[2:]))
	case "setup":
		os.Exit(setupCommand(os.Args[2:]))
	case "sync":
		os.Exit(syncCommand(os.Args[2:]))
	default:
		usage()
		os.Exit(exitUsage)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `vetter-agent — lee la copia de Vetter, nunca la escribe

  inspect --dir <ruta> [--out inspector.json] [--stat TABLA.CAMPO ...]
  pair    [--config <config.json>] [--gui | --clipboard]
  setup   [--config <config.json>]      (ventana de instalación, Windows)
  sync    --config <config.json> [--dry-run]
  version`)
}

type statList []string

func (s *statList) String() string { return strings.Join(*s, ",") }

func (s *statList) Set(value string) error {
	*s = append(*s, value)

	return nil
}

func inspect(args []string) {
	flags := flag.NewFlagSet("inspect", flag.ExitOnError)

	dir := flags.String("dir", "", "directorio con los DBF (Z: o el UNC)")
	out := flags.String("out", "inspector.json", "archivo JSON de salida")

	var stats statList

	flags.Var(&stats, "stat", "agregados de un campo: TABLA.CAMPO (repetible)")
	_ = flags.Parse(args)

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "falta --dir")
		os.Exit(2)
	}

	specs := make([]report.StatSpec, 0, len(stats))

	for _, raw := range stats {
		spec, err := report.ParseStat(raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		specs = append(specs, spec)
	}

	result, err := report.Build(*dir, specs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no se pudo leer %s: %v\n", *dir, err)
		os.Exit(1)
	}

	if err := report.Write(*out, result); err != nil {
		fmt.Fprintf(os.Stderr, "no se pudo escribir %s: %v\n", *out, err)
		os.Exit(1)
	}

	printSummary(result)
	fmt.Printf("\nReporte: %s\n", *out)
}

func printSummary(result report.Report) {
	fmt.Printf("Vetter — %s\n", result.Dir)
	fmt.Printf("%d DBF · %d errores\n\n", len(result.Files), len(result.Errors))

	for _, file := range result.Files {
		fmt.Printf("%-22s %8d reg · %5d borrados · %3d B/reg · %s %s\n",
			file.Name, file.Records.Header, file.Records.Deleted,
			file.Header.RecordBytes, file.Header.LanguageDriver, file.Header.Encoding)

		if !file.Records.Match {
			fmt.Printf("  ⚠ el header dice %d registros y el tamaño da %d\n",
				file.Records.Header, file.Records.Computed)
		}

		sort.Slice(file.Stats, func(i, j int) bool { return file.Stats[i].Field < file.Stats[j].Field })

		for _, stat := range file.Stats {
			fmt.Printf("  %-16s %d con datos · %d solo dígitos\n",
				stat.Field, stat.NonEmpty, stat.DigitsOnly)

			if len(stat.Lengths) > 0 {
				fmt.Printf("  %-16s largos: %s\n", "", formatCounts(stat.Lengths))
			}

			if len(stat.Separators) > 0 {
				fmt.Printf("  %-16s separadores: %s\n", "", formatCounts(stat.Separators))
			}

			if len(stat.PrefixLengths) > 0 {
				fmt.Printf("  %-16s prefijos antes de \"/\": %s\n", "", formatCounts(stat.PrefixLengths))
			}
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("\nErrores:")

		for _, item := range result.Errors {
			fmt.Printf("  %s: %s\n", item.File, item.Error)
		}
	}
}

func formatCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))

	for key := range counts {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	parts := make([]string, 0, len(keys))

	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, counts[key]))
	}

	return strings.Join(parts, " ")
}
