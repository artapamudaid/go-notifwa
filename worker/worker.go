package worker

import (
	"context"
	"log"
	"math/rand"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

type SendJob struct {
	Client    *whatsmeow.Client
	TargetJID types.JID
	Message   *waProto.Message
	Type      string
	Token     string // Token device untuk traceability log
}

var JobQueue = make(chan SendJob, 10000) // Buffer besar untuk queue

func StartWorkers(numWorkers int) {
	for i := 1; i <= numWorkers; i++ {
		go worker(i)
	}
	log.Printf("Started %d workers for message queue\n", numWorkers)
}

func worker(id int) {
	for job := range JobQueue {
		processJob(id, job)
	}
}

func processJob(workerID int, job SendJob) {
	// Random delay (1-5 seconds) untuk menghindari block/banned
	delay := time.Duration(rand.Intn(4000)+1000) * time.Millisecond
	time.Sleep(delay)

	// Cek apakah client masih terhubung sebelum kirim.
	// Jika client disconnect antara saat job masuk queue dan saat diproses,
	// kita skip daripada kirim ke client yang sudah mati.
	if job.Client == nil || !job.Client.IsConnected() {
		log.Printf("[Worker %d] Skipped %s message to %s (token=%s): client disconnected\n",
			workerID, job.Type, job.TargetJID, job.Token)
		job.Message = nil
		job.Client = nil
		return
	}

	_, err := job.Client.SendMessage(context.Background(), job.TargetJID, job.Message)
	if err != nil {
		log.Printf("[Worker %d] Failed to send %s message to %s (token=%s): %v\n",
			workerID, job.Type, job.TargetJID, job.Token, err)
	} else {
		log.Printf("[Worker %d] Successfully sent %s message to %s (token=%s)\n",
			workerID, job.Type, job.TargetJID, job.Token)
	}

	// Auto clean / Garbage Collection Helper
	// Melepaskan referensi memori agar struct message (yang mungkin memuat base64/media besar) bisa segera dihapus oleh GC Go
	job.Message = nil
	job.Client = nil
}
