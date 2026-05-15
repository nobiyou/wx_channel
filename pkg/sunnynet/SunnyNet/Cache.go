package SunnyNet

import (
	"github.com/qtgolang/SunnyNet/src/crypto/tls"
	"sync"
)

type caller struct {
	wg       sync.WaitGroup
	response interface{}
	err      error
}
type Group struct {
	mu     sync.Mutex
	buffer map[string]*caller
}

func (group *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	group.mu.Lock()
	if group.buffer == nil {
		group.buffer = make(map[string]*caller)
	}

	if caller, ok := group.buffer[key]; ok {
		group.mu.Unlock()
		caller.wg.Wait()
		return caller.response, caller.err
	}

	caller := new(caller)
	group.buffer[key] = caller
	group.mu.Unlock()

	caller.wg.Add(1)
	caller.response, caller.err = fn()
	caller.wg.Done()

	group.mu.Lock()
	delete(group.buffer, key)
	group.mu.Unlock()
	return caller.response, caller.err
}

type Cache struct {
	M           sync.Map
	singleGroup *Group
}

func NewCache() *Cache {
	return &Cache{
		singleGroup: &Group{},
	}
}

func (cache *Cache) GetOrStore(key string, fn func() (interface{}, error)) (interface{}, error) {
	if val, ok := cache.M.Load(key); ok {
		return val.(tls.Certificate), nil
	}

	cert, err := cache.singleGroup.Do(key, fn)
	if err != nil {
		return nil, err
	}
	cache.M.Store(key, cert)
	return cert.(tls.Certificate), nil
}

func (cache *Cache) GetCache() sync.Map {
	return cache.M
}
