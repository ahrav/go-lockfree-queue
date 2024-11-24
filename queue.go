package queue

import (
	"arena"
	"sync/atomic"
)

const maxNodes = 65536 // Maximum number of nodes to pre-allocate

// Node represents a node in the queue.
type Node struct {
	value any
	next  atomic.Pointer[Node] // *Node
	index uint16               // Position in nodes array
}

// Queue represents a concurrent FIFO queue with pre-allocated nodes.
type Queue struct {
	head     atomic.Pointer[Node] // *Node
	tail     atomic.Pointer[Node] // *Node
	freeHead atomic.Pointer[Node] // *Node
	freeTail atomic.Pointer[Node] // *Node
	nodes    []Node               // Pre-allocated nodes
	a        *arena.Arena         // Memory arena for allocation
}

// New creates a new empty queue with pre-allocated nodes.
func New() *Queue {
	mem := arena.NewArena()

	// Allocate nodes array in arena.
	nodes := arena.MakeSlice[Node](mem, maxNodes, maxNodes)

	// Initialize queue with a dummy node.
	q := &Queue{nodes: nodes, a: mem}

	// Initialize all nodes and link them in the free list.
	for i := range nodes {
		nodes[i].index = uint16(i)
		nodes[i].next.Store(nil)
	}

	// Setup initial dummy node.
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

	return q
}

// getNode gets a node from the free list.
func (q *Queue) getNode(value any) *Node {
	for {
		freeHeadPtr := q.freeHead.Load()
		if freeHeadPtr == nil {
			panic("out of nodes") // Free list is empty
		}

		nextFreePtr := freeHeadPtr.next.Load()
		if q.freeHead.CompareAndSwap(freeHeadPtr, nextFreePtr) {
			// atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&freeHeadPtr.value)), unsafe.Pointer(&value))
			// Successfully got a node from free list
			freeHeadPtr.value = value
			freeHeadPtr.next.Store(nil)
			return freeHeadPtr
		}
		// If CAS failed, try again.
	}
}

// returnNode returns a node to the free list.
func (q *Queue) returnNode(node *Node) {
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

// Enqueue adds a value to the tail of the queue.
func (q *Queue) Enqueue(value any) {
	// Get a new node from the free list
	node := q.getNode(value)

	for {
		tailPtr := q.tail.Load()       // [1] Get current tail
		nextPtr := tailPtr.next.Load() // [2] Get tail's next pointer

		// [3] Check tail hasn't changed since we read it.
		if tailPtr == q.tail.Load() {
			if nextPtr == nil {
				// [4] Tail has no next node. (we're at real tail)
				if tailPtr.next.CompareAndSwap(nil, node) {
					// [5] Successfully linked our node.
					q.tail.CompareAndSwap(tailPtr, node) // [6]
					return
				}
			} else {
				// [7] Tail is lagging - help advance it.
				q.tail.CompareAndSwap(tailPtr, nextPtr)
			}
		}
	}
}

// Dequeue removes and returns a value from the head of the queue.
func (q *Queue) Dequeue() (any, bool) {
	for {
		headPtr, tailPtr := q.head.Load(), q.tail.Load()
		nextPtr := headPtr.next.Load()

		// Check if head is still valid.
		if headPtr == q.head.Load() {
			// Is queue empty or tail falling behind?
			if headPtr == tailPtr {
				if nextPtr == nil {
					// Queue is empty.
					return nil, false
				}
				// Tail is falling behind, try to advance it.
				q.tail.CompareAndSwap(tailPtr, nextPtr)
			} else {
				// Queue has at least one item.
				if q.head.CompareAndSwap(headPtr, nextPtr) {
					value := nextPtr.value
					// Return old head node to free list.
					q.returnNode(headPtr)
					return value, true
				}
			}
		}
	}
}

// Empty returns true if the queue is empty.
func (q *Queue) Empty() bool { return q.head.Load().next.Load() == nil }

// Close releases the arena memory.
func (q *Queue) Close() { q.a.Free() }
