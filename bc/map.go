// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package bc

// Type of the data transmitted from the client to the server.
type Map map[string]string

type mapQueue struct {
	next     int
	maps     map[int]Map
	capacity int
}

func newMapQueue(capacity int) *mapQueue {
	return &mapQueue{0, make(map[int]Map), capacity}
}

func (q *mapQueue) enqueue(offset int, maps []Map) {
	if offset < q.next {
		return
	}
	if (len(q.maps) + len(maps)) > q.capacity {
		return
	}
	for i, m := range maps {
		q.maps[offset+i] = m
	}
}

func (q *mapQueue) dequeue() (m Map, ok bool) {
	if m, ok = q.maps[q.next]; ok {
		delete(q.maps, q.next)
		q.next++
	}
	return
}
