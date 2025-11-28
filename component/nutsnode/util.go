package nutsnode

import (
	"context"
	"io"
	"log/slog"

	"github.com/sirupsen/logrus"
)

var _ logrus.Hook = (*logrusSlogBridgeHook)(nil)

// logrusSlogBridgeHook is a logrus hook that bridges logrus logs to slog.
type logrusSlogBridgeHook struct {
}

func (a logrusSlogBridgeHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (a logrusSlogBridgeHook) Fire(entry *logrus.Entry) error {
	// Use entry.Context if available to preserve trace correlation
	ctx := entry.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Build slog attributes from logrus fields
	attrs := make([]any, 0, len(entry.Data)*2+2)
	attrs = append(attrs, "component", "nutsnode")
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
