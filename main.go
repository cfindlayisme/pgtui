// Command pgtui is a small terminal UI for browsing a Postgres server:
// a pgAdmin-style database/schema/table tree on the left, query results
// on the right, and an always-visible query bar at the bottom.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cfindlayisme/pgtui/config"
	"github.com/cfindlayisme/pgtui/translations"
	"github.com/cfindlayisme/pgtui/ui"
)

func main() {
	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgtui:", err)
		os.Exit(1)
	}
	translations.SetLocale(cfg.Lang)

	cfg, err = config.ResolvePassword(cfg, config.PromptPassword)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgtui:", translations.T("main.password_error", err))
		os.Exit(1)
	}

	ctx := context.Background()
	app, err := ui.NewApp(ctx, cfg.DSN(), cfg.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgtui:", translations.T("main.connect_error", err))
		os.Exit(1)
	}

	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "pgtui:", err)
		os.Exit(1)
	}
}
