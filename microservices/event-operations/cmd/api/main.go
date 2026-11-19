package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	api "nexo-event-operations/internal/httpapi"
	"nexo-event-operations/internal/worker"
	"os"
	"time"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL es obligatorio")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	runCtx, stop := context.WithCancel(context.Background())
	defer stop()
	go worker.New(pool).Run(runCtx)
	addr := os.Getenv("OPERATIONS_ADDR")
	if addr == "" {
		addr = ":8083"
	}
	log.Fatal(api.New(pool).App.Listen(addr))
}
