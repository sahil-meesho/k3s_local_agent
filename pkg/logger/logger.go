package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

type Logger interface {
	Info(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Debug(args ...interface{})
	Warn(args ...interface{})
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
}

type logger struct {
	*logrus.Entry
}

func New() Logger {
	log := logrus.New()
	log.SetOutput(os.Stdout)
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetLevel(logrus.InfoLevel)

	return &logger{
		Entry: logrus.NewEntry(log),
	}
}

func (l *logger) WithField(key string, value interface{}) Logger {
	return &logger{Entry: l.Entry.WithField(key, value)}
}

func (l *logger) WithError(err error) Logger {
	return &logger{Entry: l.Entry.WithError(err)}
}
