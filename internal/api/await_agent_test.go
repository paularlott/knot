package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A handler waiting on a space agent returns when its request ends, so an
// AI-written script's time limit holds for run-command and execute-script.
func TestAwaitAgentHonoursContext(t *testing.T) {
	ch := make(chan *int, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	reply, err := awaitAgent(ctx, ch)
	if !errors.Is(err, context.DeadlineExceeded) || reply != nil {
		t.Fatalf("got %v, %v; want the deadline", reply, err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("awaitAgent outlived its context")
	}

	n := 7
	ch <- &n
	if reply, err := awaitAgent(context.Background(), ch); err != nil || reply == nil || *reply != 7 {
		t.Errorf("got %v, %v; want the reply", reply, err)
	}

	close(ch)
	if reply, err := awaitAgent(context.Background(), ch); err != nil || reply != nil {
		t.Errorf("closed channel: got %v, %v; want nil, nil", reply, err)
	}
}
