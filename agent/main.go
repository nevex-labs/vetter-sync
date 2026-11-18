// vetter-agent: lee la copia de Vetter sin escribirla nunca.
//
// Uso:
//
//	vetter-agent inspect --dir \\Srvarchvete01\c\vetter5\Datos --out inspector.json
//	vetter-agent inspect --dir ... --stat CLIENTES.TELEFONO --stat PACIENTES.CODIGOPACI
//	vetter-agent version
//
// `inspect` es la fase 3 del SPEC: estructura, conteos y agregados sin PII.
// `pair` y `sync` van después; todavía no están implementados.
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
var version = "0.1.0"

var updatePublicKey = ""

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "inspect":
		inspect(os.Args[2:])
	case "version":
		fmt.Printf("vetter-agent %s\n", version)
	case "pair", "sync":
		fmt.Println("Todavía no implementado. Ver docs/contract.md y docs/pc-runbook.md.")
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `vetter-agent — lee la copia de Vetter, nunca la escribe

  inspect --dir <ruta> [--out inspector.json] [--stat TABLA.CAMPO ...]
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
	flags.Parse(args)

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
