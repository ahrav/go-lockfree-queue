package queue

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestQueueBasicOperations(t *testing.T) {
	t.Run("enqueue and dequeue single value", func(t *testing.T) {
		q := New[int]()

		// Test empty queue.
		assert.True(t, q.Empty(), "new queue should be empty")

		// Test enqueue.
		q.Enqueue(42)
		assert.False(t, q.Empty(), "queue should not be empty after enqueue")

		// Test dequeue.
		val, ok := q.Dequeue()
		assert.True(t, ok, "dequeue should succeed")
		assert.Equal(t, 42, val, "dequeued value should match enqueued value")
		assert.True(t, q.Empty(), "queue should be empty after dequeue")
	})

	t.Run("FIFO order", func(t *testing.T) {
		q := New[int]()

		values := []int{1, 2, 3, 4, 5}

		// Enqueue values.
		for _, v := range values {
			q.Enqueue(v)
		}

		// Dequeue and verify order
		for _, expected := range values {
			val, ok := q.Dequeue()
			assert.True(t, ok, "dequeue should succeed")
			assert.Equal(t, expected, val, "values should be dequeued in FIFO order")
		}
	})

	t.Run("dequeue empty queue", func(t *testing.T) {
		q := New[int]()

		val, ok := q.Dequeue()
		assert.False(t, ok, "dequeue on empty queue should return false")
		assert.Zero(t, val, "dequeue on empty queue should return zero value")
	})
}

func TestSingleProducerMultipleConsumers(t *testing.T) {
	q := New[int]()

	const numConsumers = 50          // Number of dequeuing workers.
	const itemsToProduce = 5_000_000 // Total number of items produced.

	var enqueuedCount, dequeuedCount uint64
	var (
		seenValues sync.Map
		wg         sync.WaitGroup
	)

	// Producer goroutine: enqueue items.
	go func() {
		for i := 1; i <= itemsToProduce; i++ {
			q.Enqueue(i)
			atomic.AddUint64(&enqueuedCount, 1)
		}
	}()

	// Consumer goroutines: dequeue items.
	wg.Add(numConsumers)
	for c := 0; c < numConsumers; c++ {
		go func() {
			defer wg.Done()
			for {
				item, _ := q.Dequeue()
				if item == 0 {
					if atomic.LoadUint64(&dequeuedCount) == itemsToProduce {
						// All items have been dequeued.
						return
					}
					continue
				}

				// Check for duplicates.
				if _, loaded := seenValues.LoadOrStore(item, true); loaded {
					t.Errorf("Duplicate item detected: %v", item)
				}

				atomic.AddUint64(&dequeuedCount, 1)
			}
		}()
	}

	// Wait for all consumers to finish.
	wg.Wait()

	// Validate that all items are dequeued exactly once.
	if atomic.LoadUint64(&enqueuedCount) != atomic.LoadUint64(&dequeuedCount) {
		t.Errorf("Mismatch between enqueued and dequeued items: enqueued=%d, dequeued=%d",
			atomic.LoadUint64(&enqueuedCount), atomic.LoadUint64(&dequeuedCount))
	}
}

func TestMultipleProducersSingleConsumer(t *testing.T) {
	q := New[int]()

	numProducers := runtime.NumCPU()
	const itemsToProduce = 5_000_000 // Total number of items produced.
	itemsPerProducer := itemsToProduce / numProducers

	var (
		enqueuedCount, dequeuedCount uint64
		wg                           sync.WaitGroup
	)

	// Producer goroutines: enqueue items.
	wg.Add(numProducers)
	for p := 0; p < numProducers; p++ {
		go func(producerID int) {
			defer wg.Done()
			start := (producerID * itemsPerProducer) + 1
			end := (producerID + 1) * itemsPerProducer
			if producerID == numProducers-1 {
				// Last producer handles any remaining items
				end = itemsToProduce
			}
			for i := start; i <= end; i++ {
				q.Enqueue(i)
				atomic.AddUint64(&enqueuedCount, 1)
			}
		}(p)
	}

	// Consumer goroutine: dequeue items.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, ok := q.Dequeue()
			if !ok {
				continue
			}

			atomic.AddUint64(&dequeuedCount, 1)
			if atomic.LoadUint64(&dequeuedCount) == itemsToProduce {
				// All items have been dequeued.
				return
			}
		}
	}()

	wg.Wait()

	// Validate that all items are dequeued exactly once.
	if atomic.LoadUint64(&enqueuedCount) != itemsToProduce {
		t.Errorf("Wrong number of items enqueued: got %d, want %d",
			atomic.LoadUint64(&enqueuedCount), itemsToProduce)
	}
	if atomic.LoadUint64(&dequeuedCount) != itemsToProduce {
		t.Errorf("Wrong number of items dequeued: got %d, want %d",
			atomic.LoadUint64(&dequeuedCount), itemsToProduce)
	}
}

func TestMultipleProducersMultipleConsumers(t *testing.T) {
	q := New[int]()

	numProducers := runtime.NumCPU() / 2
	numConsumers := runtime.NumCPU() / 2
	const totalItems = 5_000_000
	itemsPerProducer := totalItems / numProducers

	var (
		enqueuedCount, dequeuedCount uint64
		seenValues                   sync.Map
		wg                           sync.WaitGroup
	)

	// Producer goroutines.
	wg.Add(numProducers)
	for p := 0; p < numProducers; p++ {
		go func(producerID int) {
			defer wg.Done()
			start := (producerID * itemsPerProducer) + 1
			end := (producerID + 1) * itemsPerProducer
			if producerID == numProducers-1 {
				end = totalItems
			}
			for i := start; i <= end; i++ {
				q.Enqueue(i)
				atomic.AddUint64(&enqueuedCount, 1)
			}
		}(p)
	}

	// Consumer goroutines.
	wg.Add(numConsumers)
	for c := 0; c < numConsumers; c++ {
		go func() {
			defer wg.Done()
			for {
				val, ok := q.Dequeue()
				if !ok {
					if atomic.LoadUint64(&dequeuedCount) == uint64(totalItems) {
						return
					}
					continue
				}

				// Check for duplicates.
				if _, loaded := seenValues.LoadOrStore(val, true); loaded {
					t.Errorf("Duplicate value detected: %v", val)
				}

				atomic.AddUint64(&dequeuedCount, 1)
			}
		}()
	}

	// Wait with timeout.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Test completed normally.
	case <-time.After(30 * time.Second):
		t.Fatal("Test timed out")
	}

	// Validate results.
	if atomic.LoadUint64(&enqueuedCount) != uint64(totalItems) {
		t.Errorf("Wrong number of items enqueued: got %d, want %d",
			atomic.LoadUint64(&enqueuedCount), totalItems)
	}
	if atomic.LoadUint64(&dequeuedCount) != uint64(totalItems) {
		t.Errorf("Wrong number of items dequeued: got %d, want %d",
			atomic.LoadUint64(&dequeuedCount), totalItems)
	}

	// Verify queue is empty.
	if v, ok := q.Dequeue(); ok {
		t.Errorf("Queue should be empty but got value: %v", v)
	}
}

func TestABAIssueStress(t *testing.T) {
	q := New[int]()
	numGoroutines := runtime.NumCPU()
	const opsPerGoroutine = 1_000

	var wg sync.WaitGroup
	wg.Add(numGoroutines + 1)

	// Track consistency of dequeued items.
	var enqueuedCount, dequeuedCount uint64
	seenValues := sync.Map{}
	var errorsFound uint64

	// Enqueue items.
	go func() {
		defer wg.Done()
		for i := 1; i <= numGoroutines*opsPerGoroutine; i++ {
			q.Enqueue(i)
			atomic.AddUint64(&enqueuedCount, 1)
		}
	}()

	// Dequeue items in parallel.
	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				// Add some random delays to increase chance of race conditions.
				if i%100 == 0 {
					time.Sleep(time.Microsecond)
				}

				v, ok := q.Dequeue()
				if ok {
					if _, loaded := seenValues.LoadOrStore(v, goroutineID); loaded {
						atomic.AddUint64(&errorsFound, 1)
						t.Errorf("ABA issue detected: value %v dequeued multiple times", v)
					}
					atomic.AddUint64(&dequeuedCount, 1)
				}
			}
		}(g)
	}

	// Wait with timeout.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Test completed normally.
	case <-time.After(30 * time.Second):
		t.Fatal("Test timed out")
	}

	// Final validations.
	enqueued := atomic.LoadUint64(&enqueuedCount)
	dequeued := atomic.LoadUint64(&dequeuedCount)
	errors := atomic.LoadUint64(&errorsFound)

	if enqueued != dequeued {
		t.Errorf("Mismatch between enqueued and dequeued items: enqueued=%d, dequeued=%d",
			enqueued, dequeued)
	}

	if errors > 0 {
		t.Errorf("Detected %d ABA issues during test", errors)
	}

	// Validate queue is empty at the end.
	if v, ok := q.Dequeue(); ok {
		t.Errorf("Queue should be empty but got value: %v", v)
	}
}

func BenchmarkEnqueueDequeueSequential(b *testing.B) {
	q := New[int]()

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(i)
		q.Dequeue()
	}
}

func BenchmarkEnqueueDequeueParallel(b *testing.B) {
	q := New[int]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Enqueue(1)
			_, _ = q.Dequeue()
		}
	})
}

func BenchmarkProducerConsumer(b *testing.B) {
	q := New[int]()

	var wg sync.WaitGroup
	wg.Add(2)

	b.ResetTimer()

	go func() {
		defer wg.Done()
		for i := range b.N {
			q.Enqueue(i)
		}
	}()

	go func() {
		defer wg.Done()
		for range b.N {
			_, _ = q.Dequeue()
		}
	}()

	wg.Wait()
}

func BenchmarkMultipleProducersConsumers(b *testing.B) {
	q := New[int]()

	var wg sync.WaitGroup
	wg.Add(24)

	b.ResetTimer()

	for i := range 12 {
		go func(i int) {
			defer wg.Done()
			for range b.N {
				q.Enqueue(i)
			}
		}(i)
	}

	for i := range 12 {
		go func(i int) {
			defer wg.Done()
			for range b.N {
				_, _ = q.Dequeue()
			}
		}(i)
	}

	wg.Wait()
}
