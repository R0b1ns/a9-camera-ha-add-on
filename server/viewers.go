package main

import "sync"

// liveViewers counts active /dev/<uid>/live requests, split into web UI (?raw)
// and other clients (direct links). The web UI mirrors streams started by
// others; it must not keep a stream alive after the original viewer left.
type viewerStop struct {
	raw bool
	fn  func()
}

type liveViewers struct {
	mu    sync.Mutex
	raw   map[string]int
	link  map[string]int
	stops map[string]map[*viewerStop]struct{}
}

var viewers = liveViewers{
	raw: map[string]int{}, link: map[string]int{},
	stops: map[string]map[*viewerStop]struct{}{},
}

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

// register returns a function that unregisters the viewer. It also gives the
// access API a way to stop external streams that are already running.
func (v *liveViewers) register(uid string, raw bool, stop func()) func() {
	s := &viewerStop{raw: raw, fn: stop}
	v.mu.Lock()
	if v.stops == nil {
		v.stops = map[string]map[*viewerStop]struct{}{}
	}
	if v.stops[uid] == nil {
		v.stops[uid] = map[*viewerStop]struct{}{}
	}
	v.stops[uid][s] = struct{}{}
	v.mu.Unlock()
	return func() {
		v.mu.Lock()
		delete(v.stops[uid], s)
		if len(v.stops[uid]) == 0 {
			delete(v.stops, uid)
		}
		v.mu.Unlock()
	}
}

// stopDirect ends only direct-link viewers, never the add-on's own UI preview.
func (v *liveViewers) stopDirect(uid string) {
	v.mu.Lock()
	stops := make([]func(), 0, len(v.stops[uid]))
	for s := range v.stops[uid] {
		if !s.raw {
			stops = append(stops, s.fn)
		}
	}
	v.mu.Unlock()
	for _, stop := range stops {
		stop()
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
