package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

var (
	infoLogger   = log.New(os.Stderr, "INFO  ", log.LstdFlags|log.Lshortfile)
	warnLogger   = log.New(os.Stderr, "WARN  ", log.LstdFlags|log.Lshortfile)
	errorLogger  = log.New(os.Stderr, "ERROR ", log.LstdFlags|log.Lshortfile)
	debugEnabled atomic.Bool
)

func Setup(path string) (func(), error) {
	return SetupWithLevel(path, "info")
}

func SetupWithLevel(path, level string) (func(), error) {
	debugEnabled.Store(strings.EqualFold(strings.TrimSpace(level), "debug"))
	if path == "" {
		setOutput(os.Stderr)
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return nil, err
	}
	setOutput(io.MultiWriter(os.Stderr, file))
	return func() { _ = file.Close() }, nil
}

func setOutput(output io.Writer) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(output)
	infoLogger.SetOutput(output)
	warnLogger.SetOutput(output)
	errorLogger.SetOutput(output)
}
func Infof(format string, args ...any)  { infoLogger.Printf(format, args...) }
func Warnf(format string, args ...any)  { warnLogger.Printf(format, args...) }
func Errorf(format string, args ...any) { errorLogger.Printf(format, args...) }
func Debugf(format string, args ...any) {
	if debugEnabled.Load() {
		infoLogger.Printf("DEBUG "+format, args...)
	}
}
