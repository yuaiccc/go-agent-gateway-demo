package main

import (
	"log"
	"os"

	"github.com/yuaiccc/go-agent-gateway-demo/internal/gateway"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	server := gateway.NewServer()
	if err := server.Router().Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
