package smc

import "sync"

type Filter[T any] func(obj T) bool

type Ider interface {
	Id() string
}

type Pool[T Ider] struct {
	objects map[string]T
	lock sync.RWMutex
}

func (p *Pool[T]) Get(id string) (T, bool) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	obj, exist := p.objects[id]
	return obj, exist
}

func (p *Pool[T]) Put(obj T) (T, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()
	existing, exist := p.objects[obj.Id()]
	p.objects[obj.Id()] = obj
	return existing, exist
}

func (p *Pool[T]) Remove(id string) (T, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()
	existing, exist := p.objects[id]
	delete(p.objects, id)
	return existing, exist
}

func (p *Pool[T]) RemoveAll(ids []string) []T {
	p.lock.Lock()
	defer p.lock.Unlock()
	removedList := []T{}
	for _, id := range ids {
		obj, removed := p.Remove(id)
		if removed {
			removedList = append(removedList, obj)
		}
	}
	return removedList
}

func (p *Pool[T]) Find(f Filter[T]) []T {
	p.lock.RLock()
	defer p.lock.RUnlock()
	foundList := []T{}
	for _, value := range p.objects {
		if f(value) {
			foundList = append(foundList, value)
		}
	}
	return foundList
}
