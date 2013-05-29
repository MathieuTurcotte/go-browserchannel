// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

import (
	"errors"
)

var errCapacityExceeded = errors.New("queue capacity exceeded")

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

func (q *mapQueue) enqueue(offset int, maps []Map) (err error) {
	if offset < q.next {
		return
	}

	// If the queue would exceed its capacity after the new maps are enqueued,
	// return an error since it either signals that the server is overwhelmed
	// or that there is a gap in the queue.
	if (len(q.maps) + len(maps)) > q.capacity {
		return errCapacityExceeded
	}

	for i, m := range maps {
		q.maps[offset+i] = m
	}

	return
}

func (q *mapQueue) dequeue() (m Map, ok bool) {
	if m, ok = q.maps[q.next]; ok {
		delete(q.maps, q.next)
		q.next++
	}
	return
}
