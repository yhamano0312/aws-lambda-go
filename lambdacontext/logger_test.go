//go:build go1.21
// +build go1.21

// Copyright 2026 Amazon.com, Inc. or its affiliates. All Rights Reserved.

package lambdacontext

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceAttr(t *testing.T) {
	tests := []struct {
		name     string
		groups   []string
		attr     slog.Attr
		expected slog.Attr
	}{
		{
			name:     "time to timestamp",
			groups:   nil,
			attr:     slog.String(slog.TimeKey, "2025-01-09T12:00:00Z"),
			expected: slog.String("timestamp", "2025-01-09T12:00:00Z"),
		},
		{
			name:     "msg to message",
			groups:   nil,
			attr:     slog.String(slog.MessageKey, "test message"),
			expected: slog.String("message", "test message"),
		},
		{
			name:     "level unchanged",
			groups:   nil,
			attr:     slog.String(slog.LevelKey, "INFO"),
			expected: slog.String(slog.LevelKey, "INFO"),
		},
		{
			name:     "custom key unchanged",
			groups:   nil,
			attr:     slog.String("customKey", "value"),
			expected: slog.String("customKey", "value"),
		},
		{
			name:     "grouped attrs not replaced",
			groups:   []string{"group1"},
			attr:     slog.String(slog.TimeKey, "2025-01-09T12:00:00Z"),
			expected: slog.String(slog.TimeKey, "2025-01-09T12:00:00Z"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceAttr(tt.groups, tt.attr)
			assert.Equal(t, tt.expected.Key, result.Key)
			assert.Equal(t, tt.expected.Value.String(), result.Value.String())
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected slog.Level
	}{
		{"DEBUG", "DEBUG", slog.LevelDebug},
		{"INFO", "INFO", slog.LevelInfo},
		{"WARN", "WARN", slog.LevelWarn},
		{"ERROR", "ERROR", slog.LevelError},
		{"empty", "", slog.LevelInfo},
		{"INVALID", "INVALID", slog.LevelInfo},
		{"lowercase debug", "debug", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logLevel = tt.input
			result := parseLogLevel()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLogHandler_JSONFormat(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}
	baseHandler := slog.NewJSONHandler(&buf, opts)
	handler := &lambdaHandler{handler: baseHandler}

	lc := &LambdaContext{AwsRequestID: "test-request-123"}
	ctx := NewContext(context.Background(), lc)

	logger := slog.New(handler)
	logger.InfoContext(ctx, "test message", "key", "value")

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	require.NoError(t, err)

	assert.Equal(t, "INFO", logOutput["level"])
	assert.Equal(t, "test message", logOutput["message"])
	assert.Equal(t, "test-request-123", logOutput["requestId"])
	assert.Equal(t, "value", logOutput["key"])
	assert.Contains(t, logOutput, "timestamp")
	assert.NotContains(t, logOutput, "functionArn")
	assert.NotContains(t, logOutput, "tenantId")
}

func TestLogHandler_NoLambdaContext(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}
	baseHandler := slog.NewJSONHandler(&buf, opts)
	handler := &lambdaHandler{handler: baseHandler}

	ctx := context.Background()

	logger := slog.New(handler)
	logger.InfoContext(ctx, "no context message")

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	require.NoError(t, err)

	assert.Equal(t, "no context message", logOutput["message"])
	assert.NotContains(t, logOutput, "requestId")
}

func TestLogHandler_ConcurrencySafe(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}

	handler1 := &lambdaHandler{handler: slog.NewJSONHandler(&buf1, opts)}
	handler2 := &lambdaHandler{handler: slog.NewJSONHandler(&buf2, opts)}

	lc1 := &LambdaContext{AwsRequestID: "request-aaa"}
	lc2 := &LambdaContext{AwsRequestID: "request-bbb"}

	ctx1 := NewContext(context.Background(), lc1)
	ctx2 := NewContext(context.Background(), lc2)

	logger1 := slog.New(handler1)
	logger2 := slog.New(handler2)

	logger1.InfoContext(ctx1, "message 1")
	logger2.InfoContext(ctx2, "message 2")

	var output1, output2 map[string]interface{}
	require.NoError(t, json.Unmarshal(buf1.Bytes(), &output1))
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &output2))

	assert.Equal(t, "request-aaa", output1["requestId"])
	assert.Equal(t, "request-bbb", output2["requestId"])
}

func TestLogHandler_SharedHandlerConcurrencySafe(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}

	sharedHandler := &lambdaHandler{handler: slog.NewJSONHandler(&buf, opts)}
	logger := slog.New(sharedHandler)

	lc1 := &LambdaContext{AwsRequestID: "request-aaa"}
	lc2 := &LambdaContext{AwsRequestID: "request-bbb"}

	ctx1 := NewContext(context.Background(), lc1)
	ctx2 := NewContext(context.Background(), lc2)

	logger.InfoContext(ctx1, "message 1")
	logger.InfoContext(ctx2, "message 2")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)

	var output1, output2 map[string]interface{}
	require.NoError(t, json.Unmarshal(lines[0], &output1))
	require.NoError(t, json.Unmarshal(lines[1], &output2))

	assert.Equal(t, "request-aaa", output1["requestId"])
	assert.Equal(t, "request-bbb", output2["requestId"])
}

func TestLogHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}
	baseHandler := slog.NewJSONHandler(&buf, opts)
	handler := &lambdaHandler{handler: baseHandler}

	lc := &LambdaContext{AwsRequestID: "test-request"}
	ctx := NewContext(context.Background(), lc)

	logger := slog.New(handler).With("service", "test-service")
	logger.InfoContext(ctx, "test message")

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	require.NoError(t, err)

	assert.Equal(t, "test-request", logOutput["requestId"])
	assert.Equal(t, "test-service", logOutput["service"])
}

func TestLogHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}
	baseHandler := slog.NewJSONHandler(&buf, opts)
	handler := &lambdaHandler{handler: baseHandler}

	lc := &LambdaContext{AwsRequestID: "test-request"}
	ctx := NewContext(context.Background(), lc)

	logger := slog.New(handler).WithGroup("app").With("version", "1.0")
	logger.InfoContext(ctx, "test message")

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	require.NoError(t, err)

	app, ok := logOutput["app"].(map[string]interface{})
	require.True(t, ok, "expected 'app' group in output: %s", buf.String())
	assert.Equal(t, "1.0", app["version"])
	assert.Equal(t, "test-request", app["requestId"])
}

func TestLogHandler_WithFields(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}
	baseHandler := slog.NewJSONHandler(&buf, opts)

	// Create options with fields
	options := &logOptions{}
	WithFunctionARN()(options)
	WithTenantID()(options)

	handler := &lambdaHandler{
		handler: baseHandler,
		fields:  options.fields,
	}

	lc := &LambdaContext{
		AwsRequestID:       "test-request-123",
		InvokedFunctionArn: "arn:aws:lambda:us-east-1:123456789:function:test",
		TenantID:           "tenant-abc",
	}
	ctx := NewContext(context.Background(), lc)

	logger := slog.New(handler)
	logger.InfoContext(ctx, "test message")

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	require.NoError(t, err)

	assert.Equal(t, "test-request-123", logOutput["requestId"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789:function:test", logOutput["functionArn"])
	assert.Equal(t, "tenant-abc", logOutput["tenantId"])
}

func TestLogHandler_WithFieldFunctionARNOnly(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}
	baseHandler := slog.NewJSONHandler(&buf, opts)

	options := &logOptions{}
	WithFunctionARN()(options)

	handler := &lambdaHandler{
		handler: baseHandler,
		fields:  options.fields,
	}

	lc := &LambdaContext{
		AwsRequestID:       "test-request-123",
		InvokedFunctionArn: "arn:aws:lambda:us-east-1:123456789:function:test",
		TenantID:           "tenant-abc",
	}
	ctx := NewContext(context.Background(), lc)

	logger := slog.New(handler)
	logger.InfoContext(ctx, "test message")

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	require.NoError(t, err)

	assert.Equal(t, "test-request-123", logOutput["requestId"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789:function:test", logOutput["functionArn"])
	assert.NotContains(t, logOutput, "tenantId")
}

func TestLogHandler_FieldsEmpty(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceAttr,
	}
	baseHandler := slog.NewJSONHandler(&buf, opts)

	options := &logOptions{}
	WithFunctionARN()(options)
	WithTenantID()(options)

	handler := &lambdaHandler{
		handler: baseHandler,
		fields:  options.fields,
	}

	lc := &LambdaContext{
		AwsRequestID:       "test-request-123",
		InvokedFunctionArn: "",
		TenantID:           "",
	}
	ctx := NewContext(context.Background(), lc)

	logger := slog.New(handler)
	logger.InfoContext(ctx, "test message")

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	require.NoError(t, err)

	assert.Equal(t, "test-request-123", logOutput["requestId"])
	assert.NotContains(t, logOutput, "functionArn")
	assert.NotContains(t, logOutput, "tenantId")
}

func TestWithFunctionARN(t *testing.T) {
	options := &logOptions{}
	WithFunctionARN()(options)

	assert.Len(t, options.fields, 1)
	assert.Equal(t, "functionArn", options.fields[0].key)

	lc := &LambdaContext{InvokedFunctionArn: "arn:aws:lambda:us-east-1:123456789:function:test"}
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789:function:test", options.fields[0].value(lc))
}

func TestWithTenantID(t *testing.T) {
	options := &logOptions{}
	WithTenantID()(options)

	assert.Len(t, options.fields, 1)
	assert.Equal(t, "tenantId", options.fields[0].key)

	lc := &LambdaContext{TenantID: "tenant-abc"}
	assert.Equal(t, "tenant-abc", options.fields[0].value(lc))
}

func TestNewLogger(t *testing.T) {
	logger := NewLogger()
	assert.NotNil(t, logger)
}

func TestNewLogHandler(t *testing.T) {
	handler := NewLogHandler()
	assert.NotNil(t, handler)

	handlerWithOpts := NewLogHandler(WithFunctionARN(), WithTenantID())
	assert.NotNil(t, handlerWithOpts)
}
