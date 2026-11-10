// Copyright 2026 sham-cyber
// SPDX-License-Identifier: Apache-2.0

package fraudguard

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestFindingCallbackQueueIsBoundedAndDoesNotDelayRequests(t *testing.T) {
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	engine := testEngine(t, Config{
		EnableExpiredKeySignal:   true,
		FindingCallbackQueueSize: 1,
		OnFinding: func(Finding) {
			select {
			case <-callbackStarted:
			default:
				close(callbackStarted)
				<-releaseCallback
			}
		},
	})
	defer engine.Close()

	firstReturned := make(chan struct{})
	go func() {
		engine.Start(Request{
			VirtualKeyID:     "key",
			CredentialStatus: CredentialStatus{Expired: true},
		})
		close(firstReturned)
	}()
	select {
	case <-firstReturned:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("request hook waited for the finding callback")
	}
	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("finding callback worker did not start")
	}

	engine.Start(Request{VirtualKeyID: "key", CredentialStatus: CredentialStatus{Expired: true}})
	engine.Start(Request{VirtualKeyID: "key", CredentialStatus: CredentialStatus{Expired: true}})
	if got := engine.Metrics(); got.FindingCallbacksQueued != 2 || got.FindingCallbacksDropped != 1 {
		t.Fatalf("finding queue was not bounded as configured: %+v", got)
	}

	close(releaseCallback)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if got := engine.Metrics(); got.FindingCallbacksCompleted != 2 || got.FindingCallbacksFailed != 0 {
		t.Fatalf("finding queue did not drain cleanly: %+v", got)
	}
}

func TestFindingCallbackPanicIsContainedAndCounted(t *testing.T) {
	engine := testEngine(t, Config{
		EnableExpiredKeySignal: true,
		OnFinding: func(Finding) {
			panic("callback failure containing private data")
		},
	})
	engine.Start(Request{VirtualKeyID: "key", CredentialStatus: CredentialStatus{Expired: true}})
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if got := engine.Metrics(); got.FindingCallbacksFailed != 1 || got.FindingCallbacksCompleted != 0 {
		t.Fatalf("callback panic was not safely counted: %+v", got)
	}
}

func TestConcurrentFindingCallbacksAreSafe(t *testing.T) {
	const requests = 32
	var (
		mu    sync.Mutex
		calls int
	)
	engine := testEngine(t, Config{
		EnableExpiredKeySignal:   true,
		FindingCallbackQueueSize: requests,
		OnFinding: func(Finding) {
			mu.Lock()
			calls++
			mu.Unlock()
		},
	})

	var requestsDone sync.WaitGroup
	requestsDone.Add(requests)
	for i := 0; i < requests; i++ {
		go func(i int) {
			defer requestsDone.Done()
			engine.Start(Request{
				VirtualKeyID:     fmt.Sprintf("key-%d", i),
				CredentialStatus: CredentialStatus{Expired: true},
			})
		}(i)
	}
	requestsDone.Wait()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if calls != requests {
		t.Fatalf("callbacks lost under concurrent load: got=%d want=%d", calls, requests)
	}
	if got := engine.Metrics(); got.FindingCallbacksDropped != 0 || got.FindingCallbacksCompleted != requests {
		t.Fatalf("unexpected concurrent callback metrics: %+v", got)
	}
}
