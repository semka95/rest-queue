package storage

import (
	"container/list"
	"sync"
)

type Storage struct {
	mu     sync.RWMutex
	queues map[string]queue
}

type queue struct {
	items   *list.List // TODO: maybe change to slice
	waiters *list.List
}

func New() *Storage {
	return &Storage{
		sync.RWMutex{},
		make(map[string]queue),
	}
}

func (s *Storage) Get(queueName string) string {
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

func (s *Storage) Store(queueName, value string) {
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

func (s *Storage) GetChan(queueName string) *list.Element {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan string)
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

func (s *Storage) RemoveFromWait(queueName string, elem *list.Element) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.queues[queueName]
	if !ok {
		return
	}

	q.waiters.Remove(elem)
}
