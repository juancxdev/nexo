package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
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
	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		log.Fatal(err)
	}
	if v, ok := envPositiveInt32("PARTICIPATION_DB_MAX_CONNS"); ok {
		poolCfg.MaxConns = v
	}
	if v, ok := envPositiveInt32("PARTICIPATION_DB_MIN_CONNS"); ok {
		poolCfg.MinConns = v
	}
	if v, ok := envPositiveDuration("PARTICIPATION_DB_MAX_CONN_LIFETIME"); ok {
		poolCfg.MaxConnLifetime = v
	}
	if v, ok := envPositiveDuration("PARTICIPATION_DB_MAX_CONN_IDLE_TIME"); ok {
		poolCfg.MaxConnIdleTime = v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgxpool.NewWithConfig(ctx, poolCfg)
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
	securityConfig, err := api.SecurityConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	app, err := api.NewWithSecurityConfig(db, os.Getenv("EVENT_OPERATIONS_URL"), os.Getenv("NEXO_INTERNAL_SERVICE_TOKEN"), securityConfig)
	if err != nil {
		log.Fatal(err)
	}
	if strings.TrimSpace(os.Getenv("PARTICIPATION_CHALLENGE_SECRET")) == "" {
		log.Print("PARTICIPATION_CHALLENGE_SECRET is unset; v2 challenge keys are ephemeral and rotate on restart")
	}
	log.Printf("Participation API escuchando en %s", addr)
	log.Fatal(app.App.Listen(addr, fiber.ListenConfig{EnablePrintRoutes: false}))
}

func envPositiveInt32(name string) (int32, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || v <= 0 {
		log.Fatalf("%s inválido: debe ser entero positivo", name)
	}
	return int32(v), true
}

func envPositiveDuration(name string) (time.Duration, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v <= 0 {
		log.Fatalf("%s inválido: debe ser duración Go positiva", name)
	}
	return v, true
}
