package driver

import "sync"

type keyedMutex struct {
	mu    sync.Mutex
	locks map[VolumeID]*keyedLock
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[VolumeID]*keyedLock)}
}

func (m *keyedMutex) lock(key VolumeID) func() {
	m.mu.Lock()
	entry := m.locks[key]
	if entry == nil {
		entry = &keyedLock{}
		m.locks[key] = entry
	}
	entry.refs++
	m.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		m.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
}

func (m *keyedMutex) references(key VolumeID) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry := m.locks[key]; entry != nil {
		return entry.refs
	}
	return 0
}
