package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"go-notifwa/database"

	_ "github.com/mattn/go-sqlite3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// mu melindungi Clients dan statusCallbacks dari concurrent access (race condition).
// Tanpa mutex ini, akses map dari banyak goroutine sekaligus bisa menyebabkan crash
// atau (lebih berbahaya) membaca client yang salah → pesan terkirim ke nomor yang salah.
var mu sync.RWMutex

var clients = make(map[string]*whatsmeow.Client)
var statusCallbacks = make(map[string]func())
var DB *sqlstore.Container

// GetClient mengambil client secara thread-safe.
func GetClient(token string) (*whatsmeow.Client, bool) {
	mu.RLock()
	defer mu.RUnlock()
	c, ok := clients[token]
	return c, ok
}

// setClient menyimpan client secara thread-safe.
func setClient(token string, c *whatsmeow.Client) {
	mu.Lock()
	defer mu.Unlock()
	clients[token] = c
}

// deleteClient menghapus client secara thread-safe.
func deleteClient(token string) {
	mu.Lock()
	defer mu.Unlock()
	delete(clients, token)
}

// setCallback menyimpan callback secara thread-safe.
func setCallback(token string, cb func()) {
	mu.Lock()
	defer mu.Unlock()
	statusCallbacks[token] = cb
}

// getAndDeleteCallback mengambil lalu menghapus callback secara thread-safe (atomic).
func getAndDeleteCallback(token string) (func(), bool) {
	mu.Lock()
	defer mu.Unlock()
	cb, ok := statusCallbacks[token]
	if ok {
		delete(statusCallbacks, token)
	}
	return cb, ok
}

func InitWhatsApp() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	DB = container
}

func ConnectDevice(device string, qrCallback func(qrBase64 string), successCallback func(), disconnectCallback func(), errorCallback func(string)) {
	if disconnectCallback != nil {
		setCallback(device, disconnectCallback)
	}

	existingClient, exists := GetClient(device)
	if !exists || (exists && existingClient.Store.ID == nil) {
		if exists {
			existingClient.Disconnect()
			deleteClient(device)
		}

		deviceStore := DB.NewDevice()
		clientLog := waLog.Stdout("Client", "DEBUG", true)
		client := whatsmeow.NewClient(deviceStore, clientLog)

		// Salin `device` ke variabel lokal agar closure goroutine tidak
		// mengakses variabel loop yang berubah (meski di sini hanya 1 nilai,
		// ini adalah best-practice penting untuk mencegah closure capture bug).
		localDevice := device

		client.AddEventHandler(func(evt interface{}) {
			switch v := evt.(type) {
			case *events.Message:
				fmt.Println("Pesan baru:", v.Message.GetConversation())
			case *events.Disconnected:
				fmt.Printf("Device %s terputus dari WA\n", localDevice)
				deleteClient(localDevice)
				if cb, ok := getAndDeleteCallback(localDevice); ok {
					cb()
				}
				database.SetStatus(localDevice, "Disconnect")
			case *events.LoggedOut:
				fmt.Printf("Device %s logged out. Cleaning up memory...\n", localDevice)
				// Ambil client dari map secara aman sebelum disconnect
				if c, ok := GetClient(localDevice); ok {
					c.Disconnect()
				}
				deleteClient(localDevice)
				if cb, ok := getAndDeleteCallback(localDevice); ok {
					cb()
				}
				database.SetStatus(localDevice, "Disconnect")
			}
		})
		setClient(device, client)
	}

	client, _ := GetClient(device)

	// Jika device belum login, akan menghasilkan QR Code
	if client.Store.ID == nil {
		if client.IsConnected() {
			client.Disconnect()
		}
		qrChan, _ := client.GetQRChannel(context.Background())
		err := client.Connect()
		if err != nil {
			fmt.Println("Error connecting:", err)
			if errorCallback != nil {
				errorCallback(err.Error())
			}
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
			} else {
				fmt.Println("Error reconnecting:", err)
				if errorCallback != nil {
					errorCallback(err.Error())
				}
			}
		} else {
			// Jika sudah connect, langsung panggil success
			database.SetStatus(device, "Connected")
			successCallback()
		}
	}
}

func LogoutDevice(device string) error {
	client, exists := GetClient(device)
	if !exists {
		return fmt.Errorf("device %s tidak ditemukan", device)
	}

	if client.Store != nil && client.Store.ID != nil {
		if err := DB.DeleteDevice(context.Background(), client.Store); err != nil {
			fmt.Printf("Gagal hapus device %s dari SQLite: %v\n", device, err)
		} else {
			fmt.Printf("Device %s berhasil dihapus dari SQLite\n", device)
		}
	}

	client.Disconnect()
	deleteClient(device)
	getAndDeleteCallback(device) // bersihkan callback juga
	database.SetStatus(device, "Disconnect")

	return nil
}
