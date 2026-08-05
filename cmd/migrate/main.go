package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/tsee9iii/opspilot/internal/infrastructure/postgres"
	"github.com/tsee9iii/opspilot/internal/migration"
	"github.com/tsee9iii/opspilot/pkg/config"
	"github.com/tsee9iii/opspilot/sql/migrations"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	ctx := context.Background()

	if err := godotenv.Load(); err != nil {
		log.Println("godotenv: no .env file found, relying on environment variables")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("migrate: load config: %v", err)
	}

	pool, err := postgres.New(ctx, cfg)
	if err != nil {
		log.Fatalf("migrate: init postgres: %v", err)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		log.Fatalf("migrate: ping postgres: %v", err)
	}

	runner := migration.NewRunner(migrations.FS, migration.NewStorage(pool))

	switch os.Args[1] {
	case "up":
		applied, err := runner.Run(ctx)
		if err != nil {
			log.Fatalf("migrate up: %v", err)
		}
		if len(applied) == 0 {
			fmt.Println("no pending migrations")
		} else {
			for _, v := range applied {
				fmt.Printf("applied %s\n", v)
			}
		}
	case "status":
		st, err := runner.Status(ctx)
		if err != nil {
			log.Fatalf("migrate status: %v", err)
		}
		fmt.Println("Applied:")
		for _, v := range st.Applied {
			fmt.Printf("  %s\n", v)
		}
		if len(st.Applied) == 0 {
			fmt.Println("  (none)")
		}
		fmt.Println("Pending:")
		for _, v := range st.Pending {
			fmt.Printf("  %s\n", v)
		}
		if len(st.Pending) == 0 {
			fmt.Println("  (none)")
		}
	default:
		log.Fatalf("migrate: unknown command %q", os.Args[1])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: opspilot-migrate [up|status]")
}
