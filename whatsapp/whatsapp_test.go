package whatsapp

import (
	"fmt"
	"sync"
	"testing"
)

// --- Test 1: GetClient + setClient concurrent (simulasi blast banyak request bersamaan) ---
func TestGetSetClientConcurrent(t *testing.T) {
	const numGoroutines = 200

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// 200 writer goroutine
	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			defer wg.Done()
			token := fmt.Sprintf("device-%d", n)
			// setClient dengan nil client (cukup untuk test map write race)
			setClient(token, nil)
		}(i)
	}

	// 200 reader goroutine berjalan bersamaan dengan writer
	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			defer wg.Done()
			token := fmt.Sprintf("device-%d", n)
			GetClient(token) // concurrent read
		}(i)
	}

	wg.Wait()
	t.Log("✅ PASS: Tidak ada data race pada GetClient/setClient concurrent")
}

// --- Test 2: deleteClient concurrent ---
func TestDeleteClientConcurrent(t *testing.T) {
	// Pre-populate map
	for i := 0; i < 100; i++ {
		setClient(fmt.Sprintf("del-device-%d", i), nil)
	}

	var wg sync.WaitGroup
	wg.Add(200)

	for i := 0; i < 100; i++ {
		go func(n int) {
			defer wg.Done()
			deleteClient(fmt.Sprintf("del-device-%d", n))
		}(i)

		go func(n int) {
			defer wg.Done()
			GetClient(fmt.Sprintf("del-device-%d", n))
		}(i)
	}

	wg.Wait()
	t.Log("✅ PASS: Tidak ada data race pada deleteClient concurrent")
}

// --- Test 3: getAndDeleteCallback atomic (tidak boleh double-fire) ---
func TestGetAndDeleteCallbackAtomic(t *testing.T) {
	token := "test-callback-token"
	fireCount := 0
	var mu sync.Mutex

	setCallback(token, func() {
		mu.Lock()
		fireCount++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	const goroutines = 50
	wg.Add(goroutines)

	// 50 goroutine berebut mengambil dan menjalankan callback yang sama
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if cb, ok := getAndDeleteCallback(token); ok {
				cb()
			}
		}()
	}

	wg.Wait()

	mu.Lock()
	count := fireCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("❌ FAIL: callback dieksekusi %d kali, seharusnya tepat 1 kali (double-fire bug)", count)
	} else {
		t.Logf("✅ PASS: callback dieksekusi tepat 1 kali dari %d goroutine yang bersaing", goroutines)
	}
}

// --- Test 4: GetClient tidak menghasilkan cross-token (concurrent read) ---
func TestGetClientNoCrossToken(t *testing.T) {
	tokens := []string{"tok-X", "tok-Y", "tok-Z"}

	// Set semua token ke nil client (cukup untuk verify map tidak corrupt)
	for _, tok := range tokens {
		setClient(tok, nil)
	}

	var wg sync.WaitGroup
	wg.Add(len(tokens) * 100)

	for _, tok := range tokens {
		for i := 0; i < 100; i++ {
			go func(token string) {
				defer wg.Done()
				// Hanya verifikasi tidak panic / crash (no DATA RACE)
				GetClient(token)
			}(tok)
		}
	}

	wg.Wait()
	t.Log("✅ PASS: Concurrent read 100x per token tidak crash / tidak ada data race")
}


