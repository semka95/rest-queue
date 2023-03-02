package storage

import (
	"container/list"
	"sync"
)

// Store represents queue store
type Store struct {
	mu     sync.Mutex
	queues map[string]queue
}

type queue struct {
	items   *list.List
	waiters *list.List
}

// New creates Store
func New() *Store {
	return &Store{
		sync.Mutex{},
		make(map[string]queue),
	}
}

// Get takes item from given queue and returns it
func (s *Store) Get(queueName string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.queues[queueName]
	if !ok || q.items.Len() == 0 {
		return ""
	}

	elem := q.items.Remove(q.items.Front())
	s.queues[queueName] = q

	return elem.(string)
}

// Store stores item to given queue. Before storing it checks if there are any waiters,
// and if any it sends item to first waiter in the queue list
func (s *Store) Store(queueName, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.queues[queueName]
	if !ok {
		q = queue{
			list.New(),
			list.New(),
		}
	}

	if q.waiters.Len() > 0 {
		elem := q.waiters.Front()
		ch := elem.Value.(chan string)
		ch <- value
		return
	}

	q.items.PushBack(value)
	s.queues[queueName] = q
}

// AddToWaitQueue adds channel to the wait queue and returns list element
func (s *Store) AddToWaitQueue(queueName string, ch chan string) *list.Element {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.queues[queueName]
	if !ok {
		q = queue{
			list.New(),
			list.New(),
		}
	}

	elem := q.waiters.PushBack(ch)
	s.queues[queueName] = q

	return elem
}

// RemoveFromWaitQueue removes waiter from the queue list
func (s *Store) RemoveFromWaitQueue(queueName string, elem *list.Element) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.queues[queueName]
	if !ok {
		return
	}

	q.waiters.Remove(elem)
}
