package util

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Logger is the project's interface for logging
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	WithComponent(component string) Logger
}

// slogLogger is an implementation of Logger that uses slog
type slogLogger struct {
	logger   *slog.Logger
	basePath string // Base path to use for relative path calculations
}

// NewRootLogger creates a new root logger with the specified level and output
func NewRootLogger(level slog.Level, output io.Writer) Logger {
	if output == nil {
		output = os.Stdout
	}

	// Determine the workspace base path
	// This will be used to create relative paths in logs
	workspacePath := determineWorkspacePath()

	// Create handler options with custom source location
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize source information to include relative path
			if a.Key == slog.SourceKey {
				source := a.Value.Any().(*slog.Source)

				// Convert to relative path if possible
				relPath := createRelativePath(source.File, workspacePath)

				return slog.Attr{Key: "src", Value: slog.StringValue(relPath + ":" + fmt.Sprint(source.Line))}
			}
			return a
		},
	}

	// Create a text handler with our options
	handler := slog.NewTextHandler(output, opts)
	logger := slog.New(handler)

	return &slogLogger{
		logger:   logger,
		basePath: workspacePath,
	}
}

// createRelativePath generates a relative path from the absolute file path
// If the file is within the workspace, returns the relative path
// Otherwise returns the file name with a partial path for context
func createRelativePath(filePath, basePath string) string {
	// Check if the file is in the workspace
	if basePath != "" && strings.HasPrefix(filePath, basePath) {
		// Create a relative path from the workspace root
		rel, err := filepath.Rel(basePath, filePath)
		if err == nil {
			return rel
		}
	}

	// If not in workspace or error occurred, provide a reasonable amount of path context
	// by including up to 2 parent directories
	parts := strings.Split(filepath.ToSlash(filePath), "/")
	if len(parts) <= 2 {
		return filePath // Just return the full path if it's very short
	}

	// Include last 2-3 path elements for context
	startIdx := len(parts) - 3
	if startIdx < 0 {
		startIdx = 0
	}
	return filepath.Join(parts[startIdx:]...)
}

// determineWorkspacePath attempts to find the workspace root path
func determineWorkspacePath() string {
	// Try to get current working directory as a starting point
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Look for common workspace identifiers
	checkPaths := []string{
		wd,               // Current directory
		filepath.Dir(wd), // Parent directory
	}

	for _, dir := range checkPaths {
		// Check for common project root indicators
		for _, indicator := range []string{"go.mod", "LICENSE", ".git"} {
			if _, err := os.Stat(filepath.Join(dir, indicator)); err == nil {
				return dir
			}
		}
	}

	// Default to the current working directory if we couldn't detect workspace
	return wd
}

// Debug logs a debug message
func (l *slogLogger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info logs an info message
func (l *slogLogger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs a warning message
func (l *slogLogger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error logs an error message
func (l *slogLogger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// WithComponent returns a new logger with the component name added as an attribute
func (l *slogLogger) WithComponent(component string) Logger {
	return &slogLogger{
		logger:   l.logger.With("component", component),
		basePath: l.basePath,
	}
}

// DefaultRootLogger returns a default root logger configured for standard output with INFO level
func DefaultRootLogger() Logger {
	return NewRootLogger(slog.LevelInfo, os.Stdout)
}
