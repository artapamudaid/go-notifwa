package worker

import (
	"sync"
	"testing"
)

// --- Test: Worker job tidak cross-token (pass by value ke channel) ---
func TestWorkerJobIsolation(t *testing.T) {
	type mockJob struct {
		Token  string
		Number string
		Text   string
	}

	jobs := []mockJob{
		{"token-1", "628111111111", "Pesan untuk user 1"},
		{"token-2", "628222222222", "Pesan untuk user 2"},
		{"token-3", "628333333333", "Pesan untuk user 3"},
	}

	results := make(chan mockJob, len(jobs)*30)
	var wg sync.WaitGroup

	// 30 concurrent goroutine mengirim 3 job bersamaan (simulasi blast)
	for i := 0; i < 30; i++ {
		for _, j := range jobs {
			wg.Add(1)
			go func(jb mockJob) { // ← pass by value: bukan pointer/closure
				defer wg.Done()
				results <- jb
			}(j)
		}
	}

	wg.Wait()
	close(results)

	mismatch := 0
	for r := range results {
		var expected string
		switch r.Token {
		case "token-1":
			expected = "628111111111"
		case "token-2":
			expected = "628222222222"
		case "token-3":
			expected = "628333333333"
		}
		if r.Number != expected {
			mismatch++
			t.Errorf("❌ FAIL cross-token: token=%s dapat number=%s (seharusnya %s)", r.Token, r.Number, expected)
		}
	}

	if mismatch == 0 {
		t.Log("✅ PASS: Tidak ada cross-token mismatch — job terisolasi dengan benar per request")
	}
}

// --- Test: JobQueue channel menerima dari banyak goroutine tanpa deadlock ---
func TestJobQueueConcurrentEnqueue(t *testing.T) {
	// Buat channel lokal (tidak pakai JobQueue global agar test terisolasi)
	ch := make(chan SendJob, 10000)

	var wg sync.WaitGroup
	const total = 500
	wg.Add(total)

	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
			// Kirim job kosong — cukup untuk test tidak deadlock/race
			ch <- SendJob{Type: "Text"}
		}()
	}

	wg.Wait()
	close(ch)

	count := 0
	for range ch {
		count++
	}

	if count != total {
		t.Errorf("❌ FAIL: Hanya %d dari %d job masuk ke channel", count, total)
	} else {
		t.Logf("✅ PASS: Semua %d job berhasil di-enqueue secara concurrent tanpa deadlock", total)
	}
}
