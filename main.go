package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to the YAML config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	app := buildApp(cfg)

	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Printf("🚀 Anthropic Gateway on %s\n", addr)
	fmt.Printf("🧩 Providers: %d | Aggregations: %d | Client keys: %d\n",
		len(cfg.Providers), len(cfg.ModelAggregations), len(cfg.ClientKeys))

	if err := app.Listen(addr); err != nil {
		log.Fatal(err)
	}
}

// buildApp wires up the Fiber application, middleware, and routes.
func buildApp(cfg *Config) *fiber.App {
	app := fiber.New(fiber.Config{AppName: "Anthropic Gateway"})

	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"*"},
		AllowHeaders: []string{"*"},
	}))

	h := NewApp(cfg)

	app.Post("/v1/messages", h.handleMessages)
	app.Post("/v1/messages/count_tokens", h.handleCountTokens)
	app.Get("/v1/models", h.handleListModels)

	return app
}
