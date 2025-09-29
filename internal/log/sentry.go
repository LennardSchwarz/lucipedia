package log

import (
	"time"

	"github.com/getsentry/sentry-go"
	sentrylogrus "github.com/getsentry/sentry-go/logrus"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
)

// SentrySettings represents the configuration required to bootstrap Sentry.
type SentrySettings struct {
	DSN         string
	Environment string
	Release     string
}

// InitSentry wires up Sentry exception logging and connects it to the provided logrus logger.
func InitSentry(logger *logrus.Logger, settings SentrySettings) (*sentry.Hub, func(), error) {
	if settings.DSN == "" {
		return nil, func() {}, nil
	}

	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:         settings.DSN,
		Environment: settings.Environment,
		Release:     settings.Release,
	})
	if err != nil {
		return nil, nil, eris.Wrap(err, "error initializing sentry client")
	}

	hub := sentry.NewHub(client, sentry.NewScope())

	hook := sentrylogrus.NewLogHookFromClient([]logrus.Level{
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
	}, client)
	logger.AddHook(hook)

	flush := func() {
		hub.Flush(2 * time.Second)
	}

	return hub, flush, nil
}
