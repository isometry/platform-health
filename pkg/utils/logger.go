package utils

import (
	"context"
	"log/slog"
	"os"

	slogctx "github.com/veqryn/slog-context"
)

func ContextLogger(ctx context.Context, args ...any) *slog.Logger {
	return slogctx.FromCtx(ctx).With(args...)
}

func IsTTY() bool {
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
