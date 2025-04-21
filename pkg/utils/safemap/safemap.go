package safemap

import "sync"

type SafeMap[K comparable, V any] struct {
	mu *sync.RWMutex
	m  map[K]V
}

func NewSafeMap[K comparable, V any]() *SafeMap[K, V] {
	return &SafeMap[K, V]{
		m: make(map[K]V),
	}
}

func (sm *SafeMap[K, V]) set(key K, value V) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[key] = value
}

func (sm *SafeMap[K, V]) get(key K) (value V, ok bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	value, ok = sm.m[key]
	return
}

func (sm *SafeMap[K, V]) delete(key K) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.m, key)
}

func SetSafeMap[K comparable, V any](m map[K]V, mu *sync.RWMutex, key K, value V) {
	mu.Lock()
	defer mu.Unlock()
	m[key] = value
}

func GetSafeMap[K comparable, V any](m map[K]V, mu *sync.RWMutex, key K) (value V, ok bool) {
	mu.RLock()
	defer mu.RUnlock()

	value, ok = m[key]
	return
}

func DeleteSafeMap[K comparable, V any](m map[K]V, mu *sync.RWMutex, key K) {
	mu.Lock()
	defer mu.Unlock()
	delete(m, key)
}
