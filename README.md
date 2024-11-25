# go-lockfree-queue

## Purpose

This repository contains an educational implementation of Michael and Scott's non-blocking concurrent queue algorithm in Go.
The primary goal is to provide a clear, well-documented example of how to implement a lock-free data structure using Go's concurrency primitives.

## Overview

The non-blocking concurrent queue algorithm, introduced by Maged M. Michael and Michael L. Scott in their 1996 paper,
allows multiple threads to enqueue and dequeue items simultaneously without using locks.
This implementation achieves wait-free progress for enqueue operations and lock-free progress for dequeue operations.

### Key Features

1. **Lock-Free Operations**: Both enqueue and dequeue operations are implemented without using locks, allowing for high concurrency.
2. **ABA Problem Mitigation**: Instead of using counters as suggested in the original paper, this implementation uses hazard pointers as a workaround for the ABA problem.

### Example Usage

The following example demonstrates how to use the `go-lockfree-queue` library to create a queue and perform basic enqueue and dequeue operations:
```go
// Default configuration.
q1 := queue.New[string]()

// Custom configuration.
q2 := queue.New[string](
    queue.WithMaxNodes(1<<20),              // 1 million nodes
    queue.WithReclaimInterval(100 * time.Millisecond), // More frequent reclamation
)
```

#### Available Options

1. **WithMaxNodes(n int)**: Sets the maximum number of nodes in the queue.
   - Default: 65,536 (1<<16) nodes
   - Use when: You need to handle more concurrent items or want to reduce memory usage
   - Example: `queue.WithMaxNodes(1<<20)` for 1 million nodes

2. **WithReclaimInterval(d time.Duration)**: Sets how often the queue attempts to reclaim nodes.
   - Default: 5 seconds
   - Use when:
     - High throughput: Decrease interval to reclaim nodes more frequently
     - Low throughput: Increase interval to reduce CPU overhead
   - Example: `queue.WithReclaimInterval(100 * time.Millisecond)` for high-throughput scenarios


### Key Operations

#### Enqueue Operation

1. Get a node from the free list and set its value.
2. Use Compare-And-Swap (CAS) to append the new node to the tail of the queue.
3. If necessary, update the tail pointer to the newly added node.

#### Dequeue Operation

1. Check if the queue is empty (head's next is null).
2. Use CAS to remove the first non-dummy node from the head of the queue.
3. Update the head to point to the new first node.
4. Return the value from the dequeued node and return the node to the free list.

### Memory Management

- Pre-allocates a fixed number of nodes to avoid dynamic allocation during queue operations.
- Manages a free list of nodes for reuse, reducing garbage collection pressure.


## Limitations

- Fixed maximum capacity due to pre-allocated node pool.
- Intended for educational purposes and may not be suitable for production use without further testing and optimization.

## References

- Michael, M. M., & Scott, M. L. (1996). Simple, fast, and practical non-blocking and blocking concurrent queue algorithms. In Proceedings of the fifteenth annual ACM symposium on Principles of distributed computing (pp. 267-275).
https://www.cs.rochester.edu/u/scott/papers/1996_PODC_queues.pdf
