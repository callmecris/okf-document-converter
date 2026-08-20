// Package logger construye el logger estructurado (slog en JSON) usado por
// la API y los workers.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New devuelve un *slog.Logger que escribe JSON a stdout con el nivel dado
// ("debug", "info", "warn", "error"). Un nivel desconocido cae en info.
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(log)
	return log
}
