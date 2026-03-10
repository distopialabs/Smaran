package logging

import (
	"os"

	"github.com/op/go-logging"
)

var format = logging.MustStringFormatter(
	`%{color}%{time:2006-01-02 15:04:05.000} %{module} ▶ %{level:.4s}%{color:reset} %{message}`,
)

func init() {
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	backendFormatter := logging.NewBackendFormatter(backend, format)
	logging.SetBackend(backendFormatter)
}

// GetLogger returns a named logger instance.
func GetLogger(module string) *logging.Logger {
	return logging.MustGetLogger(module)
}

// SetLevel parses a level string (e.g. "DEBUG", "INFO", "WARNING", "ERROR",
// "CRITICAL") and applies it globally to all modules.
func SetLevel(level string) error {
	lvl, err := logging.LogLevel(level)
	if err != nil {
		return err
	}
	logging.SetLevel(lvl, "")
	return nil
}
