package sds

import (
	"testing"

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
