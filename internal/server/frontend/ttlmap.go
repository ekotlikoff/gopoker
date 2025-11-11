package gateway

// Credit to https://stackoverflow.com/questions/25484122/map-with-ttl-option-in-go

import (
	"errors"
	"fmt"
	"sync"
	"time"

	tableserver "github.com/ekotlikoff/gopoker/internal/server/backend"
)

type item struct {
	value      *tableserver.Player
	lastAccess int64
}

// TTLMap is a map with a TTL and prevents duplicate usernames.
type TTLMap struct {
	m         map[string]*item
	usernames map[string]struct{}
	l         sync.Mutex
}

// ErrUsernameTaken means the username is currently taken and unavailable for use.
var ErrUsernameTaken = errors.New("username taken")

// NewTTLMap creates a new map
func NewTTLMap(ln int, maxTTL int, gcFrequencySecs int) (m *TTLMap) {
	m = &TTLMap{m: make(map[string]*item, ln), usernames: make(map[string]struct{}, ln)}
	go func() {
		gcFrequency := time.Tick(time.Second * time.Duration(gcFrequencySecs))
		for now := range gcFrequency {
			m.l.Lock()
			for k, v := range m.m {
				if now.Unix()-v.lastAccess > int64(maxTTL) {
					delete(m.m, k)
					delete(m.usernames, v.value.GetName())
				}
			}
			m.l.Unlock()
		}
	}()
	return
}

// Len returns the length of the map
func (m *TTLMap) Len() int {
	m.l.Lock()
	defer m.l.Unlock()
	return len(m.m)
}

// Put puts key k and value v
func (m *TTLMap) Put(k string, v *tableserver.Player) error {
	m.l.Lock()
	defer m.l.Unlock()
	if _, ok := m.usernames[v.GetName()]; ok {
		return fmt.Errorf("%w: %s", ErrUsernameTaken, v.GetName())
	}
	_, ok := m.m[k]
	if !ok {
		it := &item{value: v}
		it.lastAccess = time.Now().Unix()
		m.m[k] = it
		m.usernames[v.GetName()] = struct{}{}
	} else {
		return fmt.Errorf("failed to put key: %s, values: %s", k, v.GetName())
	}
	return nil
}

// Get gets value for key k
func (m *TTLMap) Get(k string) (v *tableserver.Player, err error) {
	m.l.Lock()
	defer m.l.Unlock()
	if it, ok := m.m[k]; ok {
		v = it.value
		it.lastAccess = time.Now().Unix()
	} else {
		err = errors.New("failed to get")
	}
	return

}

// Refresh updates the key k to newk
func (m *TTLMap) Refresh(k, newk string) error {
	m.l.Lock()
	defer m.l.Unlock()
	it, ok := m.m[k]
	if ok {
		it.lastAccess = time.Now().Unix()
	}
	if _, newok := m.m[newk]; !newok && ok {
		m.m[newk] = it
		delete(m.m, k)
	} else {
		return errors.New("failed to refresh key")
	}
	return nil
}
