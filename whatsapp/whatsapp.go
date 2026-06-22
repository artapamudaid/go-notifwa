package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"go-notifwa/database"

	_ "github.com/mattn/go-sqlite3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var Clients = make(map[string]*whatsmeow.Client)
var DB *sqlstore.Container

func InitWhatsApp() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	DB = container
}

// Fungsi untuk memulai koneksi device tertentu dan mengirimkan event QR/Login
func ConnectDevice(device string, qrCallback func(qrBase64 string), successCallback func()) {
	// Jika belum ada di map, buat client baru
	if Clients[device] == nil {
		deviceStore := DB.NewDevice()
		clientLog := waLog.Stdout("Client", "DEBUG", true)
		client := whatsmeow.NewClient(deviceStore, clientLog)

		client.AddEventHandler(func(evt interface{}) {
			switch v := evt.(type) {
			case *events.Message:
				fmt.Println("Pesan baru:", v.Message.GetConversation())
			case *events.LoggedOut:
				// Auto clean garbage collection: Hapus client dari memory (Map) ketika device logout
				fmt.Printf("Device %s logged out. Cleaning up memory...\n", device)
				client.Disconnect()
				delete(Clients, device)
				database.SetStatus(device, "Disconnected")
			}
		})
		Clients[device] = client
	}

	client := Clients[device]

	// Jika device belum login, akan menghasilkan QR Code
	if client.Store.ID == nil {
		if client.IsConnected() {
			client.Disconnect()
		}
		qrChan, _ := client.GetQRChannel(context.Background())
		err := client.Connect()
		if err != nil {
			fmt.Println("Error connecting:", err)
			return
		}

		go func() {
			for evt := range qrChan {
				if evt.Event == "code" {
					fmt.Println("QR Baru diterima dari WA server")
					// Generate PNG Base64
					png, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
					if err == nil {
						base64Image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
						qrCallback(base64Image)
					}
				} else if evt.Event == "success" {
					fmt.Println("Scan berhasil! Memanggil successCallback...")
					database.SetStatus(device, "Connected") // <-- UPDATE DB
					successCallback()
				} else if evt.Event == "timeout" {
					fmt.Println("Scan QR Timeout...")
				}
			}
		}()
	} else {
		// Jika sudah login sebelumnya, cek apakah sudah connect
		if !client.IsConnected() {
			err := client.Connect()
			if err == nil {
				database.SetStatus(device, "Connected")
				successCallback()
			}
		} else {
			// Jika sudah connect, langsung panggil success
			database.SetStatus(device, "Connected")
			successCallback()
		}
	}
}
