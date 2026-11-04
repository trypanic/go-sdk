package logger_test

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/trypanic/go-sdk/logger"
)

func ExampleNew() {
	var output bytes.Buffer
	log := logger.New(logger.Config{Env: logger.Prod, Writer: &output})
	log.Info("service ready")
	fmt.Println(strings.Contains(output.String(), "service ready"))
	// Output: true
}
