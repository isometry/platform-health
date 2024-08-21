package utils

type Set[V any] struct {
	local map[any]V
}

func NewSet[V any]() *Set[V] {
	return &Set[V]{
		local: make(map[any]V),
	}
}

func SetFrom[V any](items ...V) *Set[V] {
	set := NewSet[V]()
	set.Add(items...)
	return set
}

func (s *Set[V]) Add(value ...V) {
	for _, v := range value {
		s.local[v] = v
	}
}

func (s *Set[V]) Remove(key V) {
	delete(s.local, key)
}

func (s *Set[V]) Contains(key V) bool {
	_, ok := s.local[key]
	return ok
}

func (s *Set[V]) Get(key V) (V, bool) {
	value, ok := s.local[key]
	return value, ok
}

func (s *Set[V]) Items() []V {
	values := make([]V, 0, len(s.local))
	for _, value := range s.local {
		values = append(values, value)
	}
	return values
}

func (s *Set[V]) Size() int {
	return len(s.local)
}

func (s *Set[V]) Clear() {
	s.local = make(map[any]V)
}
