package logging

import (
	"context"
	"io"
	"log/slog"

	"github.com/sirupsen/logrus"
)

// InitLogrus configures the global logrus logger to route all log output through slog.
// This ensures libraries that use logrus (e.g. OPA, nuts-node) participate in the
// centralized slog pipeline, including OpenTelemetry export.
func InitLogrus() {
	logrus.AddHook(&logrusSlogBridgeHook{})
	logrus.StandardLogger().SetOutput(&devNullWriter{})
}

var _ logrus.Hook = (*logrusSlogBridgeHook)(nil)

// logrusSlogBridgeHook is a logrus hook that bridges logrus logs to slog.
type logrusSlogBridgeHook struct{}

func (a logrusSlogBridgeHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (a logrusSlogBridgeHook) Fire(entry *logrus.Entry) error {
	ctx := entry.Context
	if ctx == nil {
		ctx = context.Background()
	}

	attrs := make([]any, 0, len(entry.Data)*2)
	for k, v := range entry.Data {
		attrs = append(attrs, k, v)
	}

	switch entry.Level {
	case logrus.TraceLevel, logrus.DebugLevel:
		slog.DebugContext(ctx, entry.Message, attrs...)
	case logrus.InfoLevel:
		slog.InfoContext(ctx, entry.Message, attrs...)
	case logrus.WarnLevel:
		slog.WarnContext(ctx, entry.Message, attrs...)
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		slog.ErrorContext(ctx, entry.Message, attrs...)
	default:
		slog.InfoContext(ctx, entry.Message, attrs...)
	}
	return nil
}

var _ io.Writer = (*devNullWriter)(nil)

type devNullWriter struct{}

func (d devNullWriter) Write(in []byte) (n int, _ error) {
	return len(in), nil
}

// GetLogrusLevel converts a slog level to a logrus level string.
func GetLogrusLevel(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return logrus.DebugLevel.String()
	case level <= slog.LevelInfo:
		return logrus.InfoLevel.String()
	case level <= slog.LevelWarn:
		return logrus.WarnLevel.String()
	default:
		return logrus.ErrorLevel.String()
	}
}
