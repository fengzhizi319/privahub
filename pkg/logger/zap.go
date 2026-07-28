// Package logger provides structured logging with Zap and TraceID propagation.
package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey string

// TraceIDKey is the context key for trace ID propagation.
const TraceIDKey ctxKey = "trace_id"

// InitLogger creates a production or development logger based on environment.
func InitLogger(level string, format string) (*zap.Logger, error) {
	var cfg zap.Config
	if format == "console" {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Parse log level
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)

	return cfg.Build(zap.AddCallerSkip(0))
}

// WithTraceID returns a logger with trace_id field extracted from context.
func WithTraceID(ctx context.Context, logger *zap.Logger) *zap.Logger {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		return logger.With(zap.String("trace_id", traceID))
	}
	return logger
}

// ContextWithTraceID stores trace ID into context.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}
