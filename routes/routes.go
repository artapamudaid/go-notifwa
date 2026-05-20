package routes

import (
	"go-notifwa/controllers"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2" // Tambahkan ini
)

func SetupRoutes(app *fiber.App) {
	// Middleware khusus untuk jalur WebSocket
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// Route WebSocket untuk Scan QR & Status
	app.Get("/ws/connect/:device", websocket.New(controllers.WsConnect))

	// Mimic Node.js Endpoints
	app.Post("/backend-send-text", controllers.SendText)
	app.Post("/backend-send-media", controllers.SendMedia)
	app.Post("/backend-send-poll", controllers.SendPoll)
	app.Post("/backend-getgroups", controllers.GetGroups)
}
