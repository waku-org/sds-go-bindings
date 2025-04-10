package sds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateAndCleanup(t *testing.T) {

	rm1, err := NewReliabilityManager("my-channel-id-1", "rm1")
	require.NoError(t, err)

	rm2, err := NewReliabilityManager("my-channel-id-2", "rm2")
	require.NoError(t, err)

	err = rm1.Cleanup()
	require.NoError(t, err)

	err = rm2.Cleanup()
	require.NoError(t, err)
}

func TestReset(t *testing.T) {

	rm, err := NewReliabilityManager("my-channel-id", "rm")
	require.NoError(t, err)

	err = rm.Reset()
	require.NoError(t, err)

	err = rm.Cleanup()
	require.NoError(t, err)

}
