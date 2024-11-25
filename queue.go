// Package queue implements a lock-free concurrent FIFO queue using pre-allocated nodes.
// The queue is designed for high-performance concurrent access without locks,
// making it suitable for multi-producer, multi-consumer scenarios.
//
// The queue pre-allocates a fixed number of nodes (65536 by default)
// for better memory locality and reduced allocation overhead.
// When the queue is full, attempting to enqueue will panic.
//
// Example usage:
//
//	func Example() {
//		// Create a new queue
//		q := queue.New[string]()
//
//		// Enqueue some values
//		q.Enqueue("first")
//		q.Enqueue("second")
//		q.Enqueue("third")
//
//		// Dequeue values (FIFO order)
//		if val, ok := q.Dequeue(); ok {
//			fmt.Println(val) // Prints: first
//		}
//
//		if val, ok := q.Dequeue(); ok {
//			fmt.Println(val) // Prints: second
//		}
//
//		// Check if queue is empty
//		if !q.Empty() {
//			val, _ := q.Dequeue()
//			fmt.Println(val) // Prints: 123
//		}
//	}
package queue

import (
	"runtime"
	"sync/atomic"
	"time"
)

// Node represents a node in the queue.
type Node[T any] struct {
	value T
	next  atomic.Pointer[Node[T]] // *Node
	index uint16                  // Position in nodes array
}

// hazardPtr represents a single hazard pointer,
// which is used to protect nodes from being reclaimed
// while they are being accessed by concurrent operations.
// In a lock-free queue, we need to ensure that
// nodes are not freed while other threads may still be reading from them.
// Hazard pointers provide this safety by having each thread declare which
// nodes it is currently accessing.
// When a node is removed from the queue,
// it can only be freed if no hazard pointer is protecting it.
// This prevents the ABA problem where a node could be freed and
// reallocated while a thread is still trying to access it.
type hazardPtr[T any] struct{ ptr atomic.Pointer[Node[T]] }

// hazardTable manages all hazard pointers.
type hazardTable[T any] struct{ pointers []hazardPtr[T] }

func newHazardTable[T any]() *hazardTable[T] {
	maxHazardPointers := 2 * runtime.GOMAXPROCS(0) * 2 // 2 pointers per thread + buffer
	return &hazardTable[T]{pointers: make([]hazardPtr[T], maxHazardPointers)}
}

// Acquire a free hazard pointer.
func (ht *hazardTable[T]) Acquire() (*hazardPtr[T], int) {
	for i := range ht.pointers {
		if ht.pointers[i].ptr.Load() == nil {
			return &ht.pointers[i], i
		}
	}
	panic("no free hazard pointers")
}

// Release a hazard pointer.
func (ht *hazardTable[T]) Release(index int) { ht.pointers[index].ptr.Store(nil) }

// reclamationStack is a lock-free stack for retired nodes that need to be safely reclaimed.
// We need this structure because in a lock-free queue, we can't immediately free nodes
// when they're removed - other threads might still be accessing them.
// Instead, we temporarily store "retired" nodes here until we're sure no threads are accessing them
// (verified via hazard pointers).
// Using a lock-free stack ensures that the memory
// reclamation process itself doesn't become a bottleneck by avoiding locks that could
// cause thread contention.
// This is crucial for maintaining the lock-free property of
// the overall queue implementation.
type reclamationStack[T any] struct{ head atomic.Pointer[Node[T]] }

// Push a node to the reclamation stack.
func (s *reclamationStack[T]) Push(node *Node[T]) {
	for {
		oldHead := s.head.Load()
		node.next.Store(oldHead)
		if s.head.CompareAndSwap(oldHead, node) {
			return
		}
	}
}

// Pop a node from the reclamation stack.
func (s *reclamationStack[T]) Pop() *Node[T] {
	for {
		oldHead := s.head.Load()
		if oldHead == nil {
			return nil
		}
		if s.head.CompareAndSwap(oldHead, oldHead.next.Load()) {
			return oldHead
		}
	}
}

// Queue represents a concurrent FIFO queue with pre-allocated nodes.
type Queue[T any] struct {
	head     atomic.Pointer[Node[T]] // *Node
	tail     atomic.Pointer[Node[T]] // *Node
	freeHead atomic.Pointer[Node[T]] // *Node
	freeTail atomic.Pointer[Node[T]] // *Node
	nodes    []Node[T]               // Pre-allocated nodes

	hazard  *hazardTable[T]     // Hazard pointers table
	reclaim reclamationStack[T] // Reclamation stack
}

// New creates a new empty queue with pre-allocated nodes.
func New[T any]() *Queue[T] {
	const maxNodes = 1 << 20 // Maximum number of nodes to pre-allocate

	nodes := make([]Node[T], maxNodes)

	// Initialize queue with a dummy node.
	q := &Queue[T]{
		nodes:   nodes,
		hazard:  newHazardTable[T](),
		reclaim: reclamationStack[T]{},
	}

	// Initialize all nodes and link them in the free list.
	for i := range nodes {
		nodes[i].index = uint16(i)
		nodes[i].next.Store(nil)
	}

	dummyNode := &nodes[0]

	// Setup head and tail to point to dummy node.
	q.head.Store(dummyNode)
	q.tail.Store(dummyNode)

	// Setup free list - link all nodes except dummy.
	firstFreeNode := &nodes[1]
	lastFreeNode := &nodes[maxNodes-1]

	q.freeHead.Store(firstFreeNode)
	q.freeTail.Store(lastFreeNode)

	// Link all free nodes together.
	for i := 1; i < maxNodes-1; i++ {
		nodes[i].next.Store(&nodes[i+1])
	}

	go q.reclaimRoutine() // Background goroutine to clean up reclaimed nodes

	return q
}

// Enqueue adds a value to the tail of the queue.
func (q *Queue[T]) Enqueue(value T) {
	// Get a new node from the free list.
	node := q.getNode(value)

	hp, hpIdx := q.hazard.Acquire()
	defer q.hazard.Release(hpIdx)

	for {
		tailPtr := q.tail.Load() // [1] Get current tail
		hp.ptr.Store(tailPtr)    // [2] Protect tail from reclamation

		if tailPtr != q.tail.Load() {
			continue
		}

		nextPtr := tailPtr.next.Load() // [3] Get tail's next pointer
		// [4] Check tail hasn't changed since we read it.
		if tailPtr != q.tail.Load() {
			continue
		}

		if nextPtr == nil {
			// [5] Tail has no next node. (we're at real tail)
			if tailPtr.next.CompareAndSwap(nil, node) {
				// [6] Successfully linked our node.
				q.tail.CompareAndSwap(tailPtr, node) // [7]
				return
			}
		} else {
			// [8] Tail is lagging - help advance it.
			q.tail.CompareAndSwap(tailPtr, nextPtr)
		}
	}
}

// getNode from free list.
// These nodes are hazard-free, so we can use them immediately.
func (q *Queue[T]) getNode(value T) *Node[T] {
	for {
		freeHeadPtr := q.freeHead.Load()
		if freeHeadPtr == nil {
			panic("out of nodes") // Free list is empty
		}

		nextFreePtr := freeHeadPtr.next.Load()
		if q.freeHead.CompareAndSwap(freeHeadPtr, nextFreePtr) {
			// This node is exclusively ours.
			// It can't be accessed by other threads until it's linked to the queue.
			freeHeadPtr.value = value
			freeHeadPtr.next.Store(nil)
			return freeHeadPtr
		}
		// If CAS failed, try again.
	}
}

// Dequeue removes and returns a value from the head of the queue.
func (q *Queue[T]) Dequeue() (T, bool) {
	hp1, hpIdx1 := q.hazard.Acquire()
	hp2, hpIdx2 := q.hazard.Acquire()
	defer func() {
		q.hazard.Release(hpIdx1)
		q.hazard.Release(hpIdx2)
	}()

	for {
		headPtr := q.head.Load() // [1] Get current head
		hp1.ptr.Store(headPtr)   // [2] Protect head from reclamation

		if headPtr != q.head.Load() {
			continue
		}

		tailPtr := q.tail.Load()       // [4] Get current tail
		nextPtr := headPtr.next.Load() // [5] Get head's next pointer
		hp2.ptr.Store(nextPtr)         // [6] Protect next pointer from reclamation

		// [7] Check if head is still valid.
		if headPtr != q.head.Load() {
			continue
		}

		// [8] Is queue empty or tail falling behind?
		if headPtr == tailPtr {
			if nextPtr == nil {
				// [9] Queue is empty.
				return *new(T), false
			}
			// [10] Tail is falling behind, try to advance it.
			q.tail.CompareAndSwap(tailPtr, nextPtr)
		} else {
			// [11] Queue has at least one item.
			if q.head.CompareAndSwap(headPtr, nextPtr) {
				value := nextPtr.value
				// [12] Defer reclamation of old head.
				q.deferReclamation(headPtr)
				return value, true
			}
		}

	}
}

func (q *Queue[T]) deferReclamation(node *Node[T]) { q.reclaim.Push(node) }

func (q *Queue[T]) reclaimRoutine() {
	// TODO: This should be configurable. Currently a higher value cause the benchmarks to panic
	const defaultInterval = 100 * time.Microsecond
	ticker := time.NewTicker(defaultInterval)
	defer ticker.Stop()

	for {
		<-ticker.C
		q.cleanupReclaimedNodes()
		// runtime.Gosched() // Give other goroutines a chance
	}
}

// cleanupReclaimedNodes attempts to reclaim nodes from the reclamation stack.
// It first tries to reclaim each node that is not currently in use (hazardous),
// and pushes any nodes that are still in use back onto a temporary reclaim list.
// Finally, it pushes the nodes from the temporary list back
// onto the main reclamation stack.
func (q *Queue[T]) cleanupReclaimedNodes() {
	reclaimList := reclamationStack[T]{} // Temporary stack for nodes we can't reclaim yet

	// Try to reclaim each node in the reclamation stack.
	for {
		node := q.reclaim.Pop()
		if node == nil {
			break
		}

		if !q.isNodeHazardous(node) {
			q.returnNode(node) // Safe to reclaim
		} else {
			reclaimList.Push(node) // Still hazardous, save for later
		}
	}

	// Push back nodes we couldn't reclaim.
	for {
		node := reclaimList.Pop()
		if node == nil {
			break
		}
		q.reclaim.Push(node)
	}
}

func (q *Queue[T]) isNodeHazardous(node *Node[T]) bool {
	for i := range q.hazard.pointers {
		if q.hazard.pointers[i].ptr.Load() == node {
			return true
		}
	}
	return false
}

// returnNode returns a node to the free list.
func (q *Queue[T]) returnNode(node *Node[T]) {
	for {
		// Get current free tail.
		freeTailPtr := q.freeTail.Load()

		// Link returned node to current free tail.
		node.next.Store(nil)
		freeTailPtr.next.Store(node)

		// Try to swing free tail to returned node.
		if q.freeTail.CompareAndSwap(freeTailPtr, node) {
			// Successfully linked returned node to free list.
			return
		}
		// If CAS failed, try again.
	}
}

// Empty returns true if the queue is empty.
func (q *Queue[T]) Empty() bool { return q.head.Load().next.Load() == nil }
