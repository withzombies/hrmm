package buffer

// RingBuffer stores a fixed number of float64 values in FIFO order.
// When capacity is reached, oldest values are overwritten.
type RingBuffer struct {
	data     []float64
	capacity int
	head     int // next write position
	size     int // current number of elements
}

// New creates a new RingBuffer with the specified capacity.
func New(capacity int) *RingBuffer {
	return &RingBuffer{
		data:     make([]float64, capacity),
		capacity: capacity,
	}
}

// Push adds a value to the buffer, overwriting the oldest if at capacity.
func (rb *RingBuffer) Push(value float64) {
	rb.data[rb.head] = value
	rb.head = (rb.head + 1) % rb.capacity
	if rb.size < rb.capacity {
		rb.size++
	}
}

// Values returns all values in chronological order (oldest first).
// Returns nil if the buffer is empty.
func (rb *RingBuffer) Values() []float64 {
	if rb.size == 0 {
		return nil
	}
	result := make([]float64, rb.size)
	start := (rb.head - rb.size + rb.capacity) % rb.capacity
	for i := 0; i < rb.size; i++ {
		result[i] = rb.data[(start+i)%rb.capacity]
	}
	return result
}

// Len returns the current number of elements in the buffer.
func (rb *RingBuffer) Len() int {
	return rb.size
}

// Latest returns the most recently pushed value and true,
// or 0 and false if the buffer is empty.
func (rb *RingBuffer) Latest() (float64, bool) {
	if rb.size == 0 {
		return 0, false
	}
	// head points to next write position, so latest is at head-1
	idx := (rb.head - 1 + rb.capacity) % rb.capacity
	return rb.data[idx], true
}
