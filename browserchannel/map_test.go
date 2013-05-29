// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

import (
	"testing"
)

const testMapKey = "index"

func makeTestMap(index string) Map {
	return Map{testMapKey: index}
}

func verifyDequeue(t *testing.T, queue *mapQueue, maps []Map) {
	for _, expected := range maps {
		if actual, ok := queue.dequeue(); ok {
			if expected[testMapKey] != actual[testMapKey] {
				t.Fatalf("expected to dequeue %v but got %v", expected, actual)
			}
		} else {
			t.Fatalf("expected to dequeue %v but got nothing", expected)
		}
	}
}

func verifyDequeueNothing(t *testing.T, queue *mapQueue) {
	if m, ok := queue.dequeue(); ok {
		t.Fatalf("expected to queue to be empty but got %v", m)
	}
}

func TestEnqueueInOrder(t *testing.T) {
	maps := []Map{
		makeTestMap("0"),
		makeTestMap("1"),
		makeTestMap("2"),
		makeTestMap("3")}

	queue := newMapQueue(100)

	queue.enqueue(0, maps[0:2])
	verifyDequeue(t, queue, maps[0:2])

	queue.enqueue(2, maps[2:4])
	verifyDequeue(t, queue, maps[2:4])

	verifyDequeueNothing(t, queue)
}

func TestEnqueueOutOfOrder(t *testing.T) {
	maps := []Map{
		makeTestMap("0"),
		makeTestMap("1"),
		makeTestMap("2"),
		makeTestMap("3")}

	queue := newMapQueue(100)

	queue.enqueue(2, maps[2:4])
	verifyDequeueNothing(t, queue)

	queue.enqueue(0, maps[0:2])
	verifyDequeue(t, queue, maps[0:4])

	verifyDequeueNothing(t, queue)
}

func TestEnqueueDuplicate(t *testing.T) {
	maps := []Map{
		makeTestMap("0"),
		makeTestMap("1")}

	queue := newMapQueue(100)

	queue.enqueue(0, maps[0:2])
	verifyDequeue(t, queue, maps[0:2])

	queue.enqueue(0, maps[0:2])
	verifyDequeueNothing(t, queue)
}
