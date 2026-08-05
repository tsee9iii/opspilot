package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"

	"github.com/tsee9iii/opspilot/cmd/central/token"
	"github.com/tsee9iii/opspilot/internal/bootstrap"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("godotenv: no .env file found, relying on environment variables")
	}

	if len(os.Args) > 1 && os.Args[1] == "token" {
		os.Exit(token.Run(context.Background(), os.Args[2:]))
	}

	app, err := bootstrap.New(context.Background())
	if err != nil {
		log.Fatalf("central: %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("central: %v", err)
	}
}
