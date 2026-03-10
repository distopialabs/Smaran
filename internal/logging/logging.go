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
