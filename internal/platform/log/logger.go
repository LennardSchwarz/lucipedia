package log

import (
	"strings"
	"time"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
)

// NewLogger constructs a logrus logger configured with JSON output and the provided log level.
func NewLogger(level string) (*logrus.Logger, error) {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano})
	logger.SetReportCaller(false)
	logger.SetLevel(logrus.InfoLevel)

	if level == "" {
		return logger, nil
	}

	parsedLevel, err := logrus.ParseLevel(strings.ToLower(level))
	if err != nil {
		return nil, eris.Wrapf(err, "invalid log level: %s", level)
	}

	logger.SetLevel(parsedLevel)
	return logger, nil
}

// WithFields returns a child logger entry with the supplied fields attached.
func WithFields(logger *logrus.Logger, fields logrus.Fields) *logrus.Entry {
	return logger.WithFields(fields)
}
