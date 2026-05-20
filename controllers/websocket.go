package controllers

import (
	"log"

	"go-notifwa/whatsapp" // Sesuaikan nama module di go.mod Anda jika berbeda

	"github.com/gofiber/websocket/v2"
)

type WsResponse struct {
	Event   string      `json:"event"`
	Token   string      `json:"token"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	User    interface{} `json:"user,omitempty"`
	PpUrl   string      `json:"ppUrl,omitempty"`
}

func WsConnect(c *websocket.Conn) {
	device := c.Params("device")
	log.Println("Frontend Laravel terhubung ke WebSocket untuk device:", device)

	// Callback ketika QR Code baru digenerate oleh Whatsmeow
	qrCallback := func(qrBase64 string) {
		msg := WsResponse{
			Event:   "qrcode",
			Token:   device,
			Data:    qrBase64,
			Message: "Scan QR Code",
		}
		c.WriteJSON(msg)
	}

	// Callback ketika sukses login (koneksi terbuka)
	successCallback := func() {
		client := whatsapp.Clients[device]

		name := "User"
		id := device

		// Hindari error (panic) jika Store atau ID belum sepenuhnya terisi dari server WA
		if client != nil && client.Store != nil {
			if client.Store.PushName != "" {
				name = client.Store.PushName
			}
			if client.Store.ID != nil {
				id = client.Store.ID.User
			}
		}

		user := map[string]string{
			"name": name,
			"id":   id,
		}

		msg := WsResponse{
			Event: "connection-open",
			Token: device,
			User:  user,
			PpUrl: "/assets/images/waiting.jpg",
		}
		
		err := c.WriteJSON(msg)
		if err != nil {
			log.Println("Gagal mengirim status Connected ke frontend:", err)
		} else {
			log.Println("BERHASIL: Status Connected dikirim ke frontend untuk device:", device)
		}
	}

	// Mulai koneksi Whatsmeow
	whatsapp.ConnectDevice(device, qrCallback, successCallback)

	// Menahan koneksi WS agar tetap hidup
	for {
		messageType, message, err := c.ReadMessage()
		if err != nil {
			log.Println("WebSocket disconnected:", err)
			break
		}

		if messageType == websocket.TextMessage {
			log.Println("Pesan dari Frontend:", string(message))
			// Di sini Anda bisa menangani event "LogoutDevice"
		}
	}
}
