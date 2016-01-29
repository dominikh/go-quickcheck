package queue

// The implementation of our buggy Queue that we want to test. It is
// purposefully buggy in several ways.

type Queue struct {
	r, w     int
	size     int
	elements []int
}

func New(size int) *Queue {
	return &Queue{
		size:     size,
		elements: make([]int, size),
	}
}

func (q *Queue) Add(v int) {
	q.elements[q.w] = v
	q.w = (q.w + 1) % q.size
}

func (q *Queue) Get() int {
	v := q.elements[q.r]
	q.r = (q.r + 1) % q.size
	return v
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (q *Queue) Size() int {
	// this implementation is purposefully buggy.
	return abs(q.w-q.r) % q.size
}
