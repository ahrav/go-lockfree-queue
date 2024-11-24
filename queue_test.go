package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueueBasicOperations(t *testing.T) {
	t.Run("enqueue and dequeue single value", func(t *testing.T) {
		q := New[int]()
		defer q.Close()

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
		defer q.Close()

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
		defer q.Close()

		val, ok := q.Dequeue()
		assert.False(t, ok, "dequeue on empty queue should return false")
		assert.Nil(t, val, "dequeue on empty queue should return nil")
	})
}
