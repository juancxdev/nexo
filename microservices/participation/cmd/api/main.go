package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	api "nexo-participation/internal/httpapi"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL es obligatorio")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = db.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("PARTICIPATION_ADDR")
	if addr == "" {
		addr = ":8084"
	}
	app := api.New(db, os.Getenv("EVENT_OPERATIONS_URL"), os.Getenv("NEXO_INTERNAL_SERVICE_TOKEN"))
	log.Printf("Participation API escuchando en %s", addr)
	log.Fatal(app.App.Listen(addr, fiber.ListenConfig{EnablePrintRoutes: false}))
}
