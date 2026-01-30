//go:build go1.21
// +build go1.21

package lambdacontext_test

import (
	"context"
	"log/slog"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

// ExampleNewLogger demonstrates the simplest usage of NewLogger for structured logging.
// The logger automatically injects requestId from Lambda context into each log record.
func ExampleNewLogger() {
	// Set up the Lambda-aware slog logger
	slog.SetDefault(lambdacontext.NewLogger())

	lambda.Start(func(ctx context.Context) (string, error) {
		// Use slog.InfoContext to include Lambda context in logs
		slog.InfoContext(ctx, "processing request", "action", "example")
		return "success", nil
	})
}

// ExampleNewLogHandler demonstrates using NewLogHandler for more control.
func ExampleNewLogHandler() {
	// Set up the Lambda-aware slog handler
	slog.SetDefault(slog.New(lambdacontext.NewLogHandler()))

	lambda.Start(func(ctx context.Context) (string, error) {
		slog.InfoContext(ctx, "processing request", "action", "example")
		return "success", nil
	})
}

// ExampleNewLogHandler_withOptions demonstrates NewLogHandler with additional fields.
// Use WithFunctionARN() and WithTenantID() to include extra context.
func ExampleNewLogHandler_withOptions() {
	// Set up handler with function ARN and tenant ID fields
	slog.SetDefault(slog.New(lambdacontext.NewLogHandler(
		lambdacontext.WithFunctionARN(),
		lambdacontext.WithTenantID(),
	)))

	lambda.Start(func(ctx context.Context) (string, error) {
		slog.InfoContext(ctx, "multi-tenant request", "tenant", "acme-corp")
		return "success", nil
	})
}

// ExampleWithFunctionARN demonstrates using WithFunctionARN to include the function ARN.
func ExampleWithFunctionARN() {
	// Include only function ARN
	slog.SetDefault(lambdacontext.NewLogger(
		lambdacontext.WithFunctionARN(),
	))

	lambda.Start(func(ctx context.Context) (string, error) {
		// Log output will include "functionArn" field
		slog.InfoContext(ctx, "function invoked")
		return "success", nil
	})
}
