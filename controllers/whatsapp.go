package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go-notifwa/models"
	"go-notifwa/whatsapp"
	"go-notifwa/worker"

	"github.com/gofiber/fiber/v2"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// Fungsi pembantu untuk mendeteksi format nomor / grup
func parseJID(number string) types.JID {
	if strings.HasSuffix(number, "@g.us") {
		jid, _ := types.ParseJID(number)
		return jid
	}
	if strings.HasSuffix(number, "@s.whatsapp.net") {
		jid, _ := types.ParseJID(number)
		return jid
	}
	if strings.Contains(number, "-") {
		return types.NewJID(number, types.GroupServer)
	}
	if len(number) > 0 && number[0] == '0' {
		number = "62" + number[1:]
	}
	return types.NewJID(number, types.DefaultUserServer)
}

func SendText(c *fiber.Ctx) error {
	req := new(models.SendMessageRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Number == "" || req.Text == "" || req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Token, number, and text are required",
		})
	}

	client, exists := whatsapp.Clients[req.Token]
	if !exists || !client.IsConnected() {
		return c.JSON(fiber.Map{
			"status":  false,
			"message": "Check your whatsapp connection",
		})
	}

	targetJID := parseJID(req.Number)

	// Add watermark
	watermark := fmt.Sprintf("\n\n---\n*Sent at: %s*", time.Now().Format("2006-01-02 15:04:05"))
	finalText := req.Text + watermark

	msg := &waProto.Message{
		Conversation: proto.String(finalText),
	}

	worker.JobQueue <- worker.SendJob{
		Client:    client,
		TargetJID: targetJID,
		Message:   msg,
		Type:      "Text",
	}

	return c.JSON(fiber.Map{
		"status":  true,
		"message": "Message queued successfully",
	})
}

func SendMedia(c *fiber.Ctx) error {
	req := new(models.SendMediaRequest)
	if err := c.BodyParser(req); err != nil {
		return c.JSON(fiber.Map{"status": false, "message": "Invalid request body"})
	}

	if req.Number == "" || req.Url == "" || req.Token == "" {
		return c.JSON(fiber.Map{"status": false, "message": "Token, number, and url are required"})
	}

	client, exists := whatsapp.Clients[req.Token]
	if !exists || !client.IsConnected() {
		return c.JSON(fiber.Map{"status": false, "message": "Check your whatsapp connection"})
	}

	// Unduh file media dari URL
	resp, err := http.Get(req.Url)
	if err != nil {
		return c.JSON(fiber.Map{"status": false, "message": "Gagal mendownload media dari URL"})
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.JSON(fiber.Map{"status": false, "message": "Gagal membaca isi file media"})
	}

	// Tentukan tipe media Whatsmeow berdasarkan request Type
	var mediaType whatsmeow.MediaType
	switch req.Type {
	case "image":
		mediaType = whatsmeow.MediaImage
	case "video":
		mediaType = whatsmeow.MediaVideo
	case "audio":
		mediaType = whatsmeow.MediaAudio
	default:
		mediaType = whatsmeow.MediaDocument
	}

	// Upload ke server WhatsApp
	uploaded, err := client.Upload(context.Background(), data, mediaType)
	if err != nil {
		return c.JSON(fiber.Map{"status": false, "message": "Gagal upload media ke server WA"})
	}

	msg := &waProto.Message{}
	mimetype := http.DetectContentType(data)

	// Add watermark
	watermark := fmt.Sprintf("\n\n---\n*Sent at: %s*", time.Now().Format("2006-01-02 15:04:05"))
	finalCaption := req.Caption + watermark

	// Bentuk struktur protobuf Message berdasarkan tipenya
	switch req.Type {
	case "image":
		msg.ImageMessage = &waProto.ImageMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			Caption:       proto.String(finalCaption),
		}
	case "video":
		msg.VideoMessage = &waProto.VideoMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			Caption:       proto.String(finalCaption),
		}
	case "audio":
		msg.AudioMessage = &waProto.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			PTT:           proto.Bool(false),
		}
	default:
		// Default to Document
		msg.DocumentMessage = &waProto.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileName:      proto.String(req.Filename),
			Caption:       proto.String(finalCaption),
		}
	}

	targetJID := parseJID(req.Number)

	worker.JobQueue <- worker.SendJob{
		Client:    client,
		TargetJID: targetJID,
		Message:   msg,
		Type:      "Media",
	}

	return c.JSON(fiber.Map{
		"status":  true,
		"message": "Media queued successfully",
	})
}

func GetGroups(c *fiber.Ctx) error {
	req := new(models.GetGroupsRequest)
	if err := c.BodyParser(req); err != nil {
		return c.JSON(fiber.Map{"status": false, "message": "Invalid request body"})
	}

	if req.Token == "" {
		return c.JSON(fiber.Map{"status": false, "message": "Token is required"})
	}

	client, exists := whatsapp.Clients[req.Token]
	if !exists || !client.IsConnected() {
		return c.JSON(fiber.Map{"status": false, "message": "Check your whatsapp connection"})
	}

	groups, err := client.GetJoinedGroups(context.Background())
	if err != nil {
		return c.JSON(fiber.Map{"status": false, "message": "Failed to fetch groups"})
	}

	// Menyesuaikan format JSON dari Baileys agar dibaca oleh Laravel
	result := make([]map[string]interface{}, 0)
	for _, group := range groups {
		var parts []map[string]interface{}
		// Whatsmeow di versi terbaru mungkin belum menarik data participant secara otomatis dari GetJoinedGroups.
		// Namun jika properti Participants ada isinya, kita format.
		for _, p := range group.Participants {
			parts = append(parts, map[string]interface{}{
				"id": p.JID.String(),
			})
		}

		result = append(result, map[string]interface{}{
			"id":           group.JID.String(),
			"name":         group.Name,
			"subject":      group.Name,
			"participants": parts,
		})
	}

	return c.JSON(fiber.Map{
		"status": true,
		"data":   result,
	})
}

func SendPoll(c *fiber.Ctx) error {
	req := new(models.SendPollRequest)
	if err := c.BodyParser(req); err != nil {
		return c.JSON(fiber.Map{"status": false, "message": "Invalid request body"})
	}

	if req.Number == "" || req.Name == "" || req.Token == "" {
		return c.JSON(fiber.Map{"status": false, "message": "Token, number, and name are required"})
	}

	client, exists := whatsapp.Clients[req.Token]
	if !exists || !client.IsConnected() {
		return c.JSON(fiber.Map{"status": false, "message": "Check your whatsapp connection"})
	}

	var options []string
	if err := json.Unmarshal([]byte(req.Options), &options); err != nil {
		return c.JSON(fiber.Map{"status": false, "message": "Invalid options format"})
	}

	targetJID := parseJID(req.Number)

	maxSelections := 1
	if req.Countable {
		maxSelections = 0 // 0 means multiple selections allowed (unlimited)
	}

	msg := client.BuildPollCreation(req.Name, options, maxSelections)

	worker.JobQueue <- worker.SendJob{
		Client:    client,
		TargetJID: targetJID,
		Message:   msg,
		Type:      "Poll",
	}

	return c.JSON(fiber.Map{
		"status":  true,
		"message": "Poll queued successfully",
	})
}
