package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := LoadConfig()

	baseURL := flag.String("base-url", cfg.OpenAIBaseURL, "Target OpenAI-compatible API URL")
	apiKey := flag.String("api-key", cfg.OpenAIAPIKey, "Target OpenAI API Key (can also pass via x-api-key header)")
	host := flag.String("host", cfg.Host, "Host to bind the adapter to")
	port := flag.Int("port", cfg.Port, "Port to bind the adapter to")
	flag.Parse()

	cfg.Update(*baseURL, *apiKey)
	cfg.Host = *host
	cfg.Port = *port

	app := buildApp(cfg)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	fmt.Printf("🚀 Starting Anthropic Adapter on %s\n", addr)
	fmt.Printf("🔗 Proxying to: %s\n", cfg.OpenAIBaseURL)
	fmt.Printf("🧭 API type: %s\n", cfg.APIType)

	if err := app.Listen(addr); err != nil {
		log.Fatal(err)
	}
}

// buildApp wires up the Fiber application, middleware, and routes.
func buildApp(cfg *Config) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "Anthropic Adapter",
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"*"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: false,
	}))

	handlers := NewApp(cfg)

	app.Post("/v1/messages", handlers.handleMessages)
	app.Post("/v1/messages/count_tokens", handlers.handleCountTokens)

	for _, r := range app.GetRoutes() {
		fmt.Printf("  Route: %s [%s]\n", r.Path, r.Method)
	}

	return app
}
