package main

import "sync"

// liveViewers counts active /dev/<uid>/live requests, split into web UI (?raw)
// and other clients (direct links). The web UI mirrors streams started by
// others; it must not keep a stream alive after the original viewer left.
type liveViewers struct {
	mu   sync.Mutex
	raw  map[string]int
	link map[string]int
}

var viewers = liveViewers{raw: map[string]int{}, link: map[string]int{}}

func (v *liveViewers) add(uid string, raw bool, delta int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.link
	if raw {
		m = v.raw
	}
	m[uid] += delta
	if m[uid] <= 0 {
		delete(m, uid)
	}
}

func (v *liveViewers) count(uid string, raw bool) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := v.link
	if raw {
		m = v.raw
	}
	return m[uid]
}
