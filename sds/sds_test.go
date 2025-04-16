package sds

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test basic creation, cleanup, and reset
func TestLifecycle(t *testing.T) {
	channelID := "test-lifecycle"
	rm, err := NewReliabilityManager(channelID)
	require.NoError(t, err)
	require.NotNil(t, rm, "Expected ReliabilityManager to be not nil")

	defer rm.Cleanup() // Ensure cleanup even on test failure

	err = rm.Reset()
	require.NoError(t, err)
}

// Test wrapping and unwrapping a simple message
func TestWrapUnwrap(t *testing.T) {
	channelID := "test-wrap-unwrap"
	rm, err := NewReliabilityManager(channelID)
	require.NoError(t, err)
	defer rm.Cleanup()

	originalPayload := []byte("hello reliability")
	messageID := MessageID("msg-wrap-1")

	wrappedMsg, err := rm.WrapOutgoingMessage(originalPayload, messageID)
	require.NoError(t, err)

	require.Greater(t, len(wrappedMsg), 0, "Expected non-empty wrapped message")

	// Simulate receiving the wrapped message
	unwrappedMessage, err := rm.UnwrapReceivedMessage(wrappedMsg)
	require.NoError(t, err)

	require.Equal(t, string(*unwrappedMessage.Message), string(originalPayload), "Expected unwrapped and original payloads to be equal")
	require.Equal(t, len(*unwrappedMessage.MissingDeps), 0, "Expexted to be no missing dependencies")
}

// Test dependency handling
func TestDependencies(t *testing.T) {
	channelID := "test-deps"
	rm, err := NewReliabilityManager(channelID)
	require.NoError(t, err)
	defer rm.Cleanup()

	// 1. Send message 1 (will become a dependency)
	payload1 := []byte("message one")
	msgID1 := MessageID("msg-dep-1")
	wrappedMsg1, err := rm.WrapOutgoingMessage(payload1, msgID1)
	require.NoError(t, err)

	// Simulate receiving msg1 to add it to history (implicitly acknowledges it)
	_, err = rm.UnwrapReceivedMessage(wrappedMsg1)
	require.NoError(t, err)

	// 2. Send message 2 (depends on message 1 implicitly via causal history)
	payload2 := []byte("message two")
	msgID2 := MessageID("msg-dep-2")
	wrappedMsg2, err := rm.WrapOutgoingMessage(payload2, msgID2)
	require.NoError(t, err)

	// 3. Create a new manager to simulate a different peer receiving msg2 without msg1
	rm2, err := NewReliabilityManager(channelID) // Same channel ID
	require.NoError(t, err)
	defer rm2.Cleanup()

	// 4. Unwrap message 2 on the second manager - should report msg1 as missing
	unwrappedMessage2, err := rm2.UnwrapReceivedMessage(wrappedMsg2)
	require.NoError(t, err)

	require.Greater(t, len(*unwrappedMessage2.MissingDeps), 0, "Expected missing dependencies, got none")

	foundDep1 := false
	for _, dep := range *unwrappedMessage2.MissingDeps {
		if dep == msgID1 {
			foundDep1 = true
			break
		}
	}
	require.True(t, foundDep1, "Expected missing dependency %q, got %v", msgID1, *unwrappedMessage2.MissingDeps)

	// 5. Mark the dependency as met
	err = rm2.MarkDependenciesMet([]MessageID{msgID1})
	require.NoError(t, err)
}

// Test callbacks
func TestCallbacks(t *testing.T) {
	channelID := "test-callbacks"
	rm, err := NewReliabilityManager(channelID)
	require.NoError(t, err)
	defer rm.Cleanup()

	var wg sync.WaitGroup
	receivedReady := make(map[MessageID]bool)
	receivedSent := make(map[MessageID]bool)
	receivedMissing := make(map[MessageID][]MessageID)
	syncRequested := false
	var cbMutex sync.Mutex // Protect access to callback tracking maps/vars

	callbacks := EventCallbacks{
		OnMessageReady: func(messageId MessageID) {
			fmt.Printf("Test: OnMessageReady received: %s\n", messageId)
			cbMutex.Lock()
			receivedReady[messageId] = true
			cbMutex.Unlock()
			wg.Done()
		},
		OnMessageSent: func(messageId MessageID) {
			fmt.Printf("Test: OnMessageSent received: %s\n", messageId)
			cbMutex.Lock()
			receivedSent[messageId] = true
			cbMutex.Unlock()
			wg.Done()
		},
		OnMissingDependencies: func(messageId MessageID, missingDeps []MessageID) {
			fmt.Printf("Test: OnMissingDependencies received for %s: %v\n", messageId, missingDeps)
			cbMutex.Lock()
			receivedMissing[messageId] = missingDeps
			cbMutex.Unlock()
			wg.Done()
		},
		OnPeriodicSync: func() {
			fmt.Println("Test: OnPeriodicSync received")
			cbMutex.Lock()
			syncRequested = true
			cbMutex.Unlock()
			// Don't wg.Done() here, it might be called multiple times
		},
	}

	rm.RegisterCallbacks(callbacks)

	// Start tasks AFTER registering callbacks
	err = rm.StartPeriodicTasks()
	require.NoError(t, err)

	// --- Test Scenario ---

	// 1. Send msg1
	wg.Add(1) // Expect OnMessageSent for msg1 eventually
	payload1 := []byte("callback test 1")
	msgID1 := MessageID("cb-msg-1")
	wrappedMsg1, err := rm.WrapOutgoingMessage(payload1, msgID1)
	require.NoError(t, err)

	// 2. Receive msg1 (triggers OnMessageReady for msg1, OnMessageSent for msg1 via causal history)
	wg.Add(1) // Expect OnMessageReady for msg1
	_, err = rm.UnwrapReceivedMessage(wrappedMsg1)
	require.NoError(t, err)

	// 3. Send msg2 (depends on msg1)
	wg.Add(1) // Expect OnMessageSent for msg2 eventually
	payload2 := []byte("callback test 2")
	msgID2 := MessageID("cb-msg-2")
	wrappedMsg2, err := rm.WrapOutgoingMessage(payload2, msgID2)
	require.NoError(t, err)

	// 4. Receive msg2 (triggers OnMessageReady for msg2, OnMessageSent for msg2)
	wg.Add(1) // Expect OnMessageReady for msg2
	_, err = rm.UnwrapReceivedMessage(wrappedMsg2)
	require.NoError(t, err)

	// --- Verification ---
	// Wait for expected callbacks with a timeout
	waitTimeout(&wg, 5*time.Second, t)

	cbMutex.Lock()
	defer cbMutex.Unlock()

	if !receivedReady[msgID1] {
		t.Errorf("OnMessageReady not called for %s", msgID1)
	}
	if !receivedReady[msgID2] {
		t.Errorf("OnMessageReady not called for %s", msgID2)
	}
	if !receivedSent[msgID1] {
		t.Errorf("OnMessageSent not called for %s", msgID1)
	}
	if !receivedSent[msgID2] {
		t.Errorf("OnMessageSent not called for %s", msgID2)
	}
	// We didn't explicitly test missing deps in this path
	if len(receivedMissing) > 0 {
		t.Errorf("Unexpected OnMissingDependencies calls: %v", receivedMissing)
	}
	// Periodic sync is harder to guarantee in a short test, just check if it was ever true
	if !syncRequested {
		t.Logf("Warning: OnPeriodicSync might not have been called within the test timeout")
	}
}

// Helper function to wait for WaitGroup with a timeout
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration, t *testing.T) {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		// Completed normally
	case <-time.After(timeout):
		t.Fatalf("Timed out waiting for callbacks")
	}
}
