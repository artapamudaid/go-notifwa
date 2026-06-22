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

	_, err := job.Client.SendMessage(context.Background(), job.TargetJID, job.Message)
	if err != nil {
		log.Printf("[Worker %d] Failed to send %s message to %s: %v\n", workerID, job.Type, job.TargetJID, err)
	} else {
		log.Printf("[Worker %d] Successfully sent %s message to %s\n", workerID, job.Type, job.TargetJID)
	}

	// Auto clean / Garbage Collection Helper
	// Melepaskan referensi memori agar struct message (yang mungkin memuat base64/media besar) bisa segera dihapus oleh GC Go
	job.Message = nil
	job.Client = nil
}
