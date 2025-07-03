package ferroamp

import (
	"fmt"
	"log/slog"
)

type mqttLogger struct {
	logger *slog.Logger
	level  slog.Level
}

func newMqttLogger(logger *slog.Logger, level slog.Level) *mqttLogger {
	return &mqttLogger{logger: logger, level: level}
}

func (l *mqttLogger) Println(v ...any) {
	l.print(fmt.Sprint(v...))
}

func (l *mqttLogger) Printf(format string, v ...any) {
	l.print(fmt.Sprintf(format, v...))
}

func (l *mqttLogger) print(msg string) {
	switch l.level {
	case slog.LevelError:
		l.logger.Error(msg)
	case slog.LevelWarn:
		l.logger.Warn(msg)
	case slog.LevelDebug:
		l.logger.Debug(msg)
	case slog.LevelInfo:
		l.logger.Info(msg)
	default:
		l.logger.Info(msg)
	}
}
