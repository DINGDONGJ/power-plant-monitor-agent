package buffer

import "sync"

// RingBuffer 泛型环形缓冲区
type RingBuffer[T any] struct {
	mu    sync.RWMutex
	data  []T
	size  int
	head  int
	count int
}

func NewRingBuffer[T any](size int) *RingBuffer[T] {
	return &RingBuffer[T]{
		data: make([]T, size),
		size: size,
	}
}

func (r *RingBuffer[T]) Push(item T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[r.head] = item
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

func (r *RingBuffer[T]) GetAll() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]T, r.count)
	if r.count == 0 {
		return result
	}
	start := 0
	if r.count == r.size {
		start = r.head
	}
	for i := 0; i < r.count; i++ {
		result[i] = r.data[(start+i)%r.size]
	}
	return result
}

func (r *RingBuffer[T]) GetRecent(n int) []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if n > r.count {
		n = r.count
	}
	result := make([]T, n)
	start := (r.head - n + r.size) % r.size
	for i := 0; i < n; i++ {
		result[i] = r.data[(start+i)%r.size]
	}
	return result
}

func (r *RingBuffer[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}
