package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

// InitLogger initializes the logrus logger to write to a file and console.
func InitLogger() {
	Log = logrus.New()

	// Set up the log file
	file, err := os.OpenFile("service.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		Log.Fatalf("Error opening log file: %v", err)
	}

	// Set up the console output
	console := os.Stdout

	// Use MultiWriter to write to both the file and console
	Log.SetOutput(io.MultiWriter(file, console))

	// Set log format to JSON or Text
	Log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Set log level (e.g., Info, Debug)
	Log.SetLevel(logrus.InfoLevel)
}

// CloseLogger closes the file output if it is open.
func CloseLogger() {
	if file, ok := Log.Out.(*os.File); ok {
		file.Close()
	}
}
