package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"vetter-sync/agent/internal/report"
)

// attachments cuenta los archivos que referencian ADJUNTOS y FOTOS: cuántos
// existen en el UNC, cuántos solo en OneDrive y cuántos no aparecen. Sin
// valores de campo: solo agregados.
func attachmentsCommand(args []string) int {
	flags := flag.NewFlagSet("attachments", flag.ContinueOnError)

	dir := flags.String("dir", "", "carpeta con los DBF (UNC)")
	mirror := flags.String("mirror", "", "copia de OneDrive (opcional)")
	out := flags.String("out", "attachments.json", "archivo JSON de salida")

	if err := flags.Parse(args); err != nil {
		return exitUsage
	}

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "falta --dir")
		return exitUsage
	}

	result, err := report.ScanAttachments(*dir, *mirror)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return exitError
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return exitError
	}

	if err := os.WriteFile(*out, append(encoded, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return exitError
	}

	for table, stats := range result.Tables {
		fmt.Printf("%-10s filas %d  con ruta %d  UNC %d  solo OneDrive %d  no aparece %d  max %d MB\n",
			table, stats.Rows, stats.WithPath, stats.InSource, stats.InMirrorOnly, stats.Missing,
			stats.MaxBytes/(1<<20))
	}

	fmt.Println("JSON:", *out)

	return exitOK
}
