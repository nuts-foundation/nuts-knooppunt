package nutsnode

import (
	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus"
	"io"
)

var _ logrus.Hook = (*logrusZerologBridgeHook)(nil)

// logrusZerologBridgeHook is a logrus hook that bridges logrus logs to zerolog.
type logrusZerologBridgeHook struct {
}

func (a logrusZerologBridgeHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (a logrusZerologBridgeHook) Fire(entry *logrus.Entry) error {
	entry.Data["component"] = "nutsnode"
	fields := map[string]interface{}(entry.Data)
	logger := zerolog.DefaultContextLogger
	switch entry.Level {
	case logrus.TraceLevel:
		logger.Trace().Fields(fields).Msg(entry.Message)
	case logrus.DebugLevel:
		logger.Debug().Fields(fields).Msg(entry.Message)
	case logrus.InfoLevel:
		logger.Info().Fields(fields).Msg(entry.Message)
	case logrus.WarnLevel:
		logger.Warn().Fields(fields).Msg(entry.Message)
	case logrus.ErrorLevel:
		logger.Error().Fields(fields).Msg(entry.Message)
	case logrus.FatalLevel:
		logger.Fatal().Fields(fields).Msg(entry.Message)
	case logrus.PanicLevel:
		logger.Panic().Fields(fields).Msg(entry.Message)
	default:
		// For any other level, we just log it as info.
		logger.Info().Fields(fields).Msg(entry.Message)
	}
	return nil
}

var _ io.Writer = (*devNullWriter)(nil)

type devNullWriter struct{}

func (d devNullWriter) Write(in []byte) (n int, _ error) {
	return len(in), nil
}
