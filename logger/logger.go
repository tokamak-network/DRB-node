package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

// InitLogger initializes the logrus logger to write to a file and console.
func InitLogger() {
	Log = logrus.New()

	// Set output to a file
	file, err := os.OpenFile("service.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		Log.Fatalf("Error opening log file: %v", err)
	}

	Log.Out = file

	// Set log format to JSON or Text
	Log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Set log level (e.g., Info, Debug)
	Log.SetLevel(logrus.InfoLevel)
}

// CloseLogger is not necessary with logrus as it handles file closing automatically
func CloseLogger() {
	if file, ok := Log.Out.(*os.File); ok {
		file.Close()
	}
}
