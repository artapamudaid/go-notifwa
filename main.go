package main

import (
	"log"

	"go-notifwa/database"
	"go-notifwa/routes"
	"go-notifwa/whatsapp"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
	// 1. Initialize MySQL Database
	database.InitDB()

	// Initialize WhatsApp Client
	whatsapp.InitWhatsApp()

	// Initialize Fiber App
	app := fiber.New()

	// Setup Middleware
	app.Use(logger.New())

	// Setup Routes
	routes.SetupRoutes(app)

	// Start Server
	log.Fatal(app.Listen(":3001"))
}
