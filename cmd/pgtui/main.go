// Command pgtui is a small terminal UI for browsing a Postgres server:
// a pgAdmin-style database/schema/table tree on the left, query results
// on the right, and an always-visible query bar at the bottom.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cfindlayisme/pgtui/internal/config"
	"github.com/cfindlayisme/pgtui/internal/ui"
)

func main() {
	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgtui:", err)
		os.Exit(1)
	}

	cfg, err = config.ResolvePassword(cfg, config.PromptPassword)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgtui: failed to read password:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	app, err := ui.NewApp(ctx, cfg.DSN())
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgtui: failed to connect:", err)
		os.Exit(1)
	}

	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "pgtui:", err)
		os.Exit(1)
	}
}
