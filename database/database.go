package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

var DB *sql.DB

func InitDB() {
	_ = godotenv.Load()

	dbHost := getEnv("DB_HOST", "127.0.0.1")
	dbPort := getEnv("DB_PORT", "3306")
	dbUser := getEnv("DB_USERNAME", "root")
	dbPass := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_DATABASE", "notifwa")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", dbUser, dbPass, dbHost, dbPort, dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Gagal koneksi MySQL:", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal("Database tidak merespon:", err)
	}

	DB = db
	log.Println("Berhasil terkoneksi ke MySQL Database Laravel!")
}

func SetStatus(device string, status string) {
	if DB == nil {
		return
	}
	_, err := DB.Exec("UPDATE devices SET status = ? WHERE body = ?", status, device)
	if err != nil {
		log.Println("Gagal update status di MySQL:", err)
	} else {
		log.Println("Status DB untuk", device, "diubah menjadi:", status)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
