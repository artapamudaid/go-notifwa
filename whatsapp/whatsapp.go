package whatsapp

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"go-notifwa/database"

	_ "github.com/mattn/go-sqlite3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// mu melindungi Clients dan statusCallbacks dari concurrent access (race condition).
// Tanpa mutex ini, akses map dari banyak goroutine sekaligus bisa menyebabkan crash
// atau (lebih berbahaya) membaca client yang salah → pesan terkirim ke nomor yang salah.
var mu sync.RWMutex

var clients = make(map[string]*whatsmeow.Client)
var statusCallbacks = make(map[string]func())
var reconnecting sync.Map
var DB *sqlstore.Container
var mappingDB *sql.DB

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

	mappingDB, err = sql.Open("sqlite3", "file:examplestore.db?_foreign_keys=on")
	if err != nil {
		panic(err)
	}
	mappingDB.SetMaxOpenConns(1)

	_, err = mappingDB.ExecContext(context.Background(),
		`CREATE TABLE IF NOT EXISTS token_jid_mapping (
			token TEXT PRIMARY KEY,
			jid TEXT NOT NULL
		)`)
	if err != nil {
		panic(err)
	}

	restoreSessions()
}

func saveTokenJIDMapping(token string, jid types.JID) {
	if mappingDB == nil {
		return
	}
	mappingDB.ExecContext(context.Background(),
		`INSERT OR REPLACE INTO token_jid_mapping (token, jid) VALUES ($1, $2)`,
		token, jid.String())
}

func deleteTokenJIDMapping(token string) {
	if mappingDB == nil {
		return
	}
	mappingDB.ExecContext(context.Background(),
		`DELETE FROM token_jid_mapping WHERE token=$1`, token)
}

func getJIDForToken(token string) (string, error) {
	var jidStr string
	err := mappingDB.QueryRowContext(context.Background(),
		`SELECT jid FROM token_jid_mapping WHERE token=$1`, token).Scan(&jidStr)
	if err != nil {
		return "", err
	}
	return jidStr, nil
}

func restoreSessions() {
	ctx := context.Background()
	rows, err := mappingDB.QueryContext(ctx, `SELECT token, jid FROM token_jid_mapping`)
	if err != nil {
		fmt.Println("restoreSessions: failed to query mappings:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var token, jidStr string
		if err := rows.Scan(&token, &jidStr); err != nil {
			continue
		}
		jid, err := types.ParseJID(jidStr)
		if err != nil {
			continue
		}
		device, err := DB.GetDevice(ctx, jid)
		if err != nil || device == nil {
			deleteTokenJIDMapping(token)
			continue
		}

		clientLog := waLog.Stdout("Client", "DEBUG", true)
		client := whatsmeow.NewClient(device, clientLog)
		attachEventHandlers(client, token)

		setClient(token, client)

		go func(t string, c *whatsmeow.Client) {
			err := c.Connect()
			if err != nil {
				fmt.Printf("restoreSessions: reconnect failed for %s: %v\n", t, err)
				go autoReconnect(t)
				return
			}
			database.SetStatus(t, "Connected")
			fmt.Printf("restoreSessions: device %s reconnected\n", t)
		}(token, client)
	}
}

func attachEventHandlers(client *whatsmeow.Client, localDevice string) {
	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			fmt.Println("Pesan baru:", v.Message.GetConversation())
		case *events.Disconnected:
			fmt.Printf("Device %s terputus dari WA, memulai auto-reconnect...\n", localDevice)
			database.SetStatus(localDevice, "Disconnect")
			if cb, ok := getAndDeleteCallback(localDevice); ok {
				cb()
			}
			go autoReconnect(localDevice)
		case *events.LoggedOut:
			fmt.Printf("Device %s logged out. Cleaning up memory...\n", localDevice)
			if c, ok := GetClient(localDevice); ok {
				c.Disconnect()
			}
			deleteClient(localDevice)
			deleteTokenJIDMapping(localDevice)
			reconnecting.Delete(localDevice)
			if cb, ok := getAndDeleteCallback(localDevice); ok {
				cb()
			}
			database.SetStatus(localDevice, "Disconnect")
		}
	})
}

func autoReconnect(device string) {
	if _, loaded := reconnecting.LoadOrStore(device, true); loaded {
		fmt.Printf("Device %s sudah dalam proses reconnect, skip\n", device)
		return
	}
	defer reconnecting.Delete(device)

	jidStr, err := getJIDForToken(device)
	if err != nil {
		fmt.Printf("Device %s: tidak ditemukan JID, hentikan reconnect\n", device)
		return
	}

	jid, err := types.ParseJID(jidStr)
	if err != nil {
		fmt.Printf("Device %s: JID invalid, hentikan reconnect\n", device)
		return
	}

	maxRetries := 6
	baseDelay := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if _, ok := GetClient(device); !ok {
			fmt.Printf("Device %s sudah dihapus dari memori, hentikan reconnect\n", device)
			return
		}

		currentClient, _ := GetClient(device)
		if currentClient != nil && currentClient.IsConnected() {
			fmt.Printf("Device %s sudah terhubung kembali\n", device)
			database.SetStatus(device, "Connected")
			return
		}

		delay := baseDelay * time.Duration(1<<(attempt-1))
		fmt.Printf("Device %s: reconnect attempt %d/%d, menunggu %v...\n", device, attempt, maxRetries, delay)
		time.Sleep(delay)

		dev, devErr := DB.GetDevice(context.Background(), jid)
		if devErr != nil || dev == nil {
			fmt.Printf("Device %s: gagal load device dari DB: %v\n", device, devErr)
			continue
		}

		clientLog := waLog.Stdout("Client", "DEBUG", true)
		newClient := whatsmeow.NewClient(dev, clientLog)
		attachEventHandlers(newClient, device)

		if err := newClient.Connect(); err != nil {
			fmt.Printf("Device %s: reconnect attempt %d gagal: %v\n", device, attempt, err)
			continue
		}

		if oldClient, ok := GetClient(device); ok {
			oldClient.Disconnect()
		}
		setClient(device, newClient)

		fmt.Printf("Device %s berhasil reconnect pada attempt %d\n", device, attempt)
		database.SetStatus(device, "Connected")
		return
	}

	fmt.Printf("Device %s gagal reconnect setelah %d attempt, menghentikan client\n", device, maxRetries)
	if c, ok := GetClient(device); ok {
		c.Disconnect()
	}
	deleteClient(device)
	database.SetStatus(device, "Disconnect")
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

		var deviceStore *store.Device

		jidStr, err := getJIDForToken(device)
		if err == nil {
			jid, parseErr := types.ParseJID(jidStr)
			if parseErr == nil {
				existingDevice, devErr := DB.GetDevice(context.Background(), jid)
				if devErr == nil && existingDevice != nil {
					deviceStore = existingDevice
					fmt.Printf("ConnectDevice: found existing session for %s (JID: %s)\n", device, jidStr)
				}
			}
		}

		if deviceStore == nil {
			deviceStore = DB.NewDevice()
		}

		clientLog := waLog.Stdout("Client", "DEBUG", true)
		client := whatsmeow.NewClient(deviceStore, clientLog)

		localDevice := device
		attachEventHandlers(client, localDevice)

		setClient(device, client)
	}

	client, _ := GetClient(device)

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
					png, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
					if err == nil {
						base64Image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
						qrCallback(base64Image)
					}
				} else if evt.Event == "success" {
					fmt.Println("Scan berhasil! Memanggil successCallback...")
					if client.Store != nil && client.Store.ID != nil {
						saveTokenJIDMapping(device, *client.Store.ID)
					}
					database.SetStatus(device, "Connected")
					successCallback()
				} else if evt.Event == "timeout" {
					fmt.Println("Scan QR Timeout...")
				}
			}
		}()
	} else {
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
	deleteTokenJIDMapping(device)
	getAndDeleteCallback(device)
	database.SetStatus(device, "Disconnect")

	return nil
}
