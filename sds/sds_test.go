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
