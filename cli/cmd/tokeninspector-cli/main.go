package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"tokeninspector-cli/internal/cli"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, cli.ErrUsage) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	return runWithTime(ctx, args, stdout, stderr, time.Now())
}

func runWithTime(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	if len(args) == 0 {
		return cli.ErrUsage
	}

	switch args[0] {
	case "table":
		return cli.RunTable(ctx, args[1:], stdout, stderr, now)
	case "help", "--help", "-h":
		fmt.Fprintln(stdout, cli.ErrUsage)
		return nil
	default:
		return cli.RunInteractive(ctx, args, stdout, stderr, now)
	}
}
