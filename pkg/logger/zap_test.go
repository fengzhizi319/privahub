package logger

import (
	"context"
	"testing"
)

func TestInitLogger_Console(t *testing.T) {
	log, err := InitLogger("info", "console")
	if err != nil {
		t.Fatalf("InitLogger(console) failed: %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
	_ = log.Sync()
}

func TestInitLogger_JSON(t *testing.T) {
	log, err := InitLogger("debug", "json")
	if err != nil {
		t.Fatalf("InitLogger(json) failed: %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
	_ = log.Sync()
}

func TestInitLogger_InvalidLevel(t *testing.T) {
	// Invalid level should default to InfoLevel
	log, err := InitLogger("invalid-level", "console")
	if err != nil {
		t.Fatalf("InitLogger with invalid level should not fail: %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
	_ = log.Sync()
}

func TestInitLogger_AllLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error", "dpanic", "panic", "fatal"}
	for _, level := range levels {
		log, err := InitLogger(level, "console")
		if err != nil {
			t.Errorf("InitLogger(%q) failed: %v", level, err)
			continue
		}
		if log == nil {
			t.Errorf("InitLogger(%q) returned nil logger", level)
			continue
		}
		_ = log.Sync()
	}
}

func TestContextWithTraceID(t *testing.T) {
	ctx := context.Background()
	traceID := "test-trace-12345"

	ctx = ContextWithTraceID(ctx, traceID)

	// Verify the trace ID is stored in context
	val := ctx.Value(TraceIDKey)
	if val == nil {
		t.Fatal("expected trace ID in context")
	}
	if val.(string) != traceID {
		t.Errorf("expected trace ID %q, got %q", traceID, val.(string))
	}
}

func TestWithTraceID(t *testing.T) {
	log, _ := InitLogger("info", "console")
	defer log.Sync()

	ctx := ContextWithTraceID(context.Background(), "trace-abc")
	loggerWithTrace := WithTraceID(ctx, log)

	if loggerWithTrace == nil {
		t.Fatal("expected non-nil logger with trace ID")
	}
	// The logger should have the trace_id field
	// We can't easily inspect the fields, but we can verify it doesn't panic
	loggerWithTrace.Info("test message with trace")
}

func TestWithTraceID_EmptyContext(t *testing.T) {
	log, _ := InitLogger("info", "console")
	defer log.Sync()

	// Empty context should return the original logger
	loggerWithoutTrace := WithTraceID(context.Background(), log)
	if loggerWithoutTrace == nil {
		t.Fatal("expected non-nil logger")
	}
	loggerWithoutTrace.Info("test message without trace")
}

func TestWithTraceID_EmptyTraceID(t *testing.T) {
	log, _ := InitLogger("info", "console")
	defer log.Sync()

	// Empty trace ID should return the original logger
	ctx := ContextWithTraceID(context.Background(), "")
	loggerWithEmptyTrace := WithTraceID(ctx, log)
	if loggerWithEmptyTrace == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestTraceIDKey_Const(t *testing.T) {
	// Verify TraceIDKey is a ctxKey type
	var key ctxKey = TraceIDKey
	if string(key) != "trace_id" {
		t.Errorf("expected TraceIDKey to be 'trace_id', got %q", string(key))
	}
}
