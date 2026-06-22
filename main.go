package main

import (
	"log"

	"go-notifwa/database"
	"go-notifwa/routes"
	"go-notifwa/whatsapp"
	"go-notifwa/worker"

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

	// Initialize Worker Pool
	worker.StartWorkers(5) // Create 5 background workers

	// Setup Middleware
	app.Use(logger.New())

	// Setup Routes
	routes.SetupRoutes(app)

	// Start Server
	log.Fatal(app.Listen(":8088"))
}
