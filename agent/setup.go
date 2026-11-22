package main

import (
	"context"
	_ "embed"
	"flag"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"vetter-sync/agent/internal/secret"
	"vetter-sync/agent/internal/syncer"
	"vetter-sync/agent/internal/update"
)

//go:embed instalar-tarea.ps1
var taskScript []byte

// setupCommand abre la ventana de instalación: pegar el secreto, actualizar el
// agente y registrar la tarea, sin PowerShell ni flags.
func setupCommand(args []string) int {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	configPath := flags.String("config", defaultConfigPath(), "config.json de la PC")

	if err := flags.Parse(args); err != nil {
		return exitUsage
	}

	cfg, cfgErr := syncer.LoadConfig(*configPath)
	dir := filepath.Dir(*configPath)

	actions := secret.SetupActions{
		Status:  func() string { return setupStatus(*configPath, cfgErr) },
		Pair:    func(token string) string { return setupPair(cfg, cfgErr, token) },
		Update:  func() string { return setupUpdate(dir) },
		Install: func() string { return setupTask(dir) },
	}

	if err := secret.RunSetup(actions); err != nil {
		secret.Notify(err.Error(), true)

		return exitError
	}

	return exitOK
}

func setupStatus(configPath string, cfgErr error) string {
	lines := []string{"Agente " + version}

	if cfgErr != nil {
		lines = append(lines, "config: "+configPath, "  problema: "+cfgErr.Error())
	} else {
		lines = append(lines, "config: "+configPath)
	}

	if _, path, err := secret.Load(secret.Candidates()); err == nil {
		lines = append(lines, "secreto: guardado en "+path)
	} else {
		lines = append(lines, "secreto: falta")
	}

	return strings.Join(lines, "\r\n")
}

func setupPair(cfg syncer.Config, cfgErr error, token string) string {
	if err := secret.Validate(token); err != nil {
		return err.Error()
	}

	if cfgErr != nil {
		return "Falta el config.json: " + cfgErr.Error()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text, _ := pairWith(ctx, cfg, token)

	return text
}

// setupUpdate baja los dos binarios del último release y los deja en su lugar.
// La tarea puede estar corriendo: Windows permite renombrar el ejecutable en uso
// y la corrida siguiente ya usa el nuevo.
func setupUpdate(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client := &http.Client{Timeout: 60 * time.Second}

	release, err := latestRelease(ctx, client)
	if err != nil {
		return "No se pudo consultar el release: " + err.Error()
	}

	if !update.Newer(release.Version, version) {
		return "Ya está la última versión (" + version + ")."
	}

	var lines []string

	for _, name := range []string{"vetter-agent.exe", "vetter-agent-task.exe"} {
		target := filepath.Join(dir, name)

		if err := update.Download(ctx, client, release, name, target+".new"); err != nil {
			lines = append(lines, name+": no se pudo bajar ("+err.Error()+")")

			return strings.Join(lines, "\r\n")
		}

		if err := update.Rotate(target); err != nil {
			lines = append(lines, name+": no se pudo instalar ("+err.Error()+")")

			return strings.Join(lines, "\r\n")
		}

		lines = append(lines, name+": listo")
	}

	return "Actualizado a " + release.Version + ". La próxima corrida la usa.\r\n" +
		strings.Join(lines, "\r\n")
}

// setupTask escribe el script incluido y lo abre: la contraseña de Windows la
// pide el diálogo del sistema, nunca pasa por este binario.
func setupTask(dir string) string {
	scriptPath := filepath.Join(dir, "instalar-tarea.ps1")

	if err := os.WriteFile(scriptPath, taskScript, 0o600); err != nil {
		return "No se pudo escribir el script: " + err.Error()
	}

	// El script lo escribimos nosotros en una carpeta que controlamos; no viene
	// de afuera.
	cmd := exec.CommandContext(context.Background(), "powershell", "-ExecutionPolicy", "Bypass", "-File", scriptPath) //nolint:gosec
	if err := cmd.Start(); err != nil {
		return "No se pudo abrir PowerShell: " + err.Error()
	}

	go func() { _ = cmd.Wait() }()

	return "Se abrió la ventana de la tarea. Pedirá la cuenta y la contraseña de Windows."
}
