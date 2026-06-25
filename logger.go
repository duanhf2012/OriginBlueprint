package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const maxLogFileBytes int64 = 5 * 1024 * 1024

var appLogger = &fileLogger{}

type fileLogger struct {
	mu   sync.Mutex
	path string
}

func initAppLogger() {
	path := logFilePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	appLogger.mu.Lock()
	appLogger.path = path
	appLogger.mu.Unlock()
	logInfo("app", "logger initialized: "+path)
}

func logFilePath() string {
	if override := strings.TrimSpace(os.Getenv("ORIGIN_BLUEPRINT_LOG_PATH")); override != "" {
		return override
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		if executable, execErr := os.Executable(); execErr == nil {
			dir = filepath.Dir(executable)
		} else {
			dir = "."
		}
	}
	return filepath.Join(dir, "OriginBlueprint", "logs", "origin-blueprint.log")
}

func logInfo(context, message string) {
	appLogger.write("INFO", context, message, "")
}

func logError(context string, err error) {
	if err == nil {
		return
	}
	appLogger.write("ERROR", context, err.Error(), "")
}

func logPanic(context string, value interface{}) {
	appLogger.write("PANIC", context, fmt.Sprint(value), string(debug.Stack()))
}

func (l *fileLogger) write(level, context, message, stack string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.path == "" {
		l.path = logFilePath()
		_ = os.MkdirAll(filepath.Dir(l.path), 0755)
	}
	l.rotateLocked()
	line := fmt.Sprintf("%s [%s] %s: %s\n", time.Now().Format("2006-01-02 15:04:05.000"), level, context, message)
	if strings.TrimSpace(stack) != "" {
		line += strings.TrimRight(stack, "\r\n") + "\n"
	}
	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line)
}

func (l *fileLogger) rotateLocked() {
	info, err := os.Stat(l.path)
	if err != nil || info.Size() < maxLogFileBytes {
		return
	}
	backup := l.path + ".1"
	_ = os.Remove(backup)
	_ = os.Rename(l.path, backup)
}

func (a *App) LogClientError(level, message, stack, context string) {
	level = strings.ToUpper(strings.TrimSpace(level))
	if level == "" {
		level = "ERROR"
	}
	if context == "" {
		context = "frontend"
	}
	appLogger.write(level, context, message, stack)
}
