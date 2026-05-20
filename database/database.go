package database

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

var DB *sql.DB

func InitDB() {
	// Karena go-notifwa ada di sebelah folder notifwa, kita baca .env dari notifwa
	err := godotenv.Load("../notifwa/.env")
	if err != nil {
		log.Println("Peringatan: Gagal load file .env (mungkin lokasinya berbeda)")
	}

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USERNAME")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_DATABASE")

	// Format DSN MySQL: user:password@tcp(host:port)/dbname
	dsn := dbUser + ":" + dbPass + "@tcp(" + dbHost + ":" + dbPort + ")/" + dbName

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

// SetStatus mengupdate status device di tabel 'devices'
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
