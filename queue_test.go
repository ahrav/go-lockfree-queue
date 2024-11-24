package queue

import (
	"sync"
	"testing"

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
