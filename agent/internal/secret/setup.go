package secret

// SetupActions son las acciones de la ventana de instalación. Cada una devuelve
// el texto que se muestra abajo; la ventana no sabe de red, de tareas ni de
// versiones.
type SetupActions struct {
	// Pair valida el secreto con el servidor y lo guarda cifrado.
	Pair func(token string) string
	// Update baja e instala el último release del agente.
	Update func() string
	// Install registra la tarea programada.
	Install func() string
	// Status es lo que se muestra al abrir: versión, config, si hay secreto.
	Status func() string
}
