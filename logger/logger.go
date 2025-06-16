package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

var logger *logrus.Logger

// InitLogger initializes the logrus logger to write to a file and console.
func InitLogger() *logrus.Logger {
	if logger != nil {
		return logger
	}

	logger = logrus.New()

	// Set up the log file
	file, err := os.OpenFile("service.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Fatalf("Error opening log file: %v", err)
	}

	// Set up the console output
	console := os.Stdout

	// Use MultiWriter to write to both the file and console
	logger.SetOutput(io.MultiWriter(file, console))

	// Set log format to JSON or Text
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Set log level (e.g., Info, Debug)
	logger.SetLevel(logrus.InfoLevel)

	return logger
}

// CloseLogger closes the file output if it is open.
func CloseLogger() {
	if file, ok := logger.Out.(*os.File); ok {
		file.Close()
	}
}

func Info(args ...interface{}) {
	InitLogger().Info(args...)
}

func Infof(format string, args ...interface{}) {
	InitLogger().Infof(format, args...)
}

func Debug(args ...interface{}) {
	InitLogger().Debug(args...)
}

func Debugf(format string, args ...interface{}) {
	InitLogger().Debugf(format, args...)
}

func Warn(args ...interface{}) {
	InitLogger().Warn(args...)
}

func Warnf(format string, args ...interface{}) {
	InitLogger().Warnf(format, args...)
}

func Error(args ...interface{}) {
	InitLogger().Error(args...)
}

func Errorf(format string, args ...interface{}) {
	InitLogger().Errorf(format, args...)
}

func Fatal(args ...interface{}) {
	InitLogger().Fatal(args...)
}

func Fatalf(format string, args ...interface{}) {
	InitLogger().Fatalf(format, args...)
}
