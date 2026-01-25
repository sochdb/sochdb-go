package sochdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNamespaceTypes tests that namespace types are exported
func TestNamespaceTypes(t *testing.T) {
	var _ *Namespace
	var _ *Collection
	var _ NamespaceConfig
	var _ CollectionConfig
	assert.Equal(t, DistanceMetric("cosine"), DistanceMetricCosine)
}

// TestQueueTypes tests that queue types are exported
func TestQueueTypes(t *testing.T) {
	var _ *PriorityQueue
	var _ QueueConfig
	assert.Equal(t, TaskState("pending"), TaskStatePending)
}

// TestSDKVersion tests the SDK version
func TestSDKVersion(t *testing.T) {
	assert.Equal(t, "0.4.1", Version)
}
