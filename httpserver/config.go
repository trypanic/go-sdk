package httpserver

import (
	"fmt"
	"time"
)

const DateTimeLayout = "2006-01-02 15:04:05"

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host string
	Port int

	// ReadTimeout is the maximum duration for reading requests.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration for writing responses.
	WriteTimeout time.Duration
}

// Address returns the server address in host:port form.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
