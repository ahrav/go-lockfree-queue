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
2. **ABA Problem Mitigation**: Instead of using counters as suggested in the original paper, this implementation uses Go's unsafe pointers as a workaround for the ABA problem.
3. **Memory Management**: Utilizes Go's memory arenas to manage the free list, optimizing memory allocation and deallocation. (Maybe?)

## Implementation Details

### Data Structures

1. **Node**: Represents a node in the queue.
   - `value`: The stored value (of type `any`)
   - `next`: Pointer to the next node (using `atomic.Pointer`)
   - `index`: Position in the pre-allocated nodes array

2. **Queue**: The main queue structure.
   - `head`, `tail`: Pointers to the head and tail nodes
   - `freeHead`, `freeTail`: Pointers to manage the free list
   - `nodes`: Pre-allocated array of nodes
   - `a`: Pointer to the memory arena

### Key Operations

#### Initialization

- Creates a new queue with a pre-allocated array of nodes in a memory arena.
- Sets up an initial dummy node and links all other nodes in the free list.

#### Node Management

- `getNode`: Atomically retrieves a node from the free list.
- `returnNode`: Atomically returns a node to the free list.

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

- Uses Go's `arena` package for efficient memory allocation.
- Pre-allocates a fixed number of nodes to avoid dynamic allocation during queue operations.
- Manages a free list of nodes for reuse, reducing garbage collection pressure.


## Limitations

- Fixed maximum capacity due to pre-allocated node pool.
- Uses unsafe pointers, which requires careful handling to avoid memory safety issues.
- Intended for educational purposes and may not be suitable for production use without further testing and optimization.

## Usage

TODO

## References

- Michael, M. M., & Scott, M. L. (1996). Simple, fast, and practical non-blocking and blocking concurrent queue algorithms. In Proceedings of the fifteenth annual ACM symposium on Principles of distributed computing (pp. 267-275).
https://www.cs.rochester.edu/u/scott/papers/1996_PODC_queues.pdf
