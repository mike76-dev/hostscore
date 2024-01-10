package persist

import (
	"log"
	"os"

	"github.com/mike76-dev/hostscore/internal/build"
)

// Logger is a wrapper for log.Logger.
type Logger struct {
	*log.Logger
}

// printCommitHash logs build.GitRevision at startup.
func printCommitHash(logger *log.Logger) {
	if build.GitRevision != "" {
		logger.Printf("[STARTUP] commit hash %v\n", build.GitRevision)
	} else {
		logger.Println("[STARTUP] unknown commit hash")
	}
}

// NewFileLogger returns a logger that logs to logFilename. The file is opened
// in append mode, and created if it does not exist.
func NewFileLogger(logFilename string) (*Logger, error) {
	logFile, err := os.OpenFile(logFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	logger := log.New(logFile, "", log.LstdFlags|log.Lshortfile)
	printCommitHash(logger)
	return &Logger{logger}, nil
}

func (logger *Logger) Close() {
	logger.Println("[SHUTDOWN] logging has terminated")
}
