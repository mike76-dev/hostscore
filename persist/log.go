package persist

import (
	"github.com/mike76-dev/hostscore/internal/build"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// printCommitHash logs build.GitRevision at startup.
func printCommitHash(logger *zap.Logger) {
	if build.GitRevision != "" {
		logger.Sugar().Infof("STARTUP: commit hash %v", build.GitRevision)
	} else {
		logger.Sugar().Info("STARTUP: unknown commit hash")
	}
}

// NewFileLogger returns a logger that logs to logFilename.
func NewFileLogger(logFilename string, level zapcore.Level) (*zap.Logger, func(), error) {
	writer, closeFn, err := zap.Open(logFilename)
	if err != nil {
		return nil, nil, err
	}

	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.RFC3339TimeEncoder
	config.StacktraceKey = ""
	fileEncoder := zapcore.NewJSONEncoder(config)

	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, level),
	)

	logger := zap.New(
		core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)

	printCommitHash(logger)

	return logger, func() {
		logger.Sugar().Info("SHUTDOWN: logging terminated")
		logger.Sync()
		closeFn()
	}, nil
}
