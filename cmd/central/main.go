package main

import (
	"context"
	"log"

	"github.com/joho/godotenv"

	"github.com/tsee9iii/opspilot/internal/bootstrap"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("godotenv: no .env file found, relying on environment variables")
	}

	app, err := bootstrap.New(context.Background())
	if err != nil {
		log.Fatalf("central: %v", err)
	}

	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("central: %v", err)
	}
}
