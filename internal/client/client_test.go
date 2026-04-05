package client

import (
	"math/rand"
	"strconv"
	"testing"

  "gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

func TestInitializeLocalClient(t *testing.T) {
	// Generate a random hash for testing
	hash := strconv.Itoa(rand.Int())

	cli := initializeLocalClient(hash)

	assert.Equal(t, cli.runtimeHash, hash)
}

func TestInitializeDockerClient(t *testing.T) {
	// Generate a random hash for testing
	hash := strconv.Itoa(rand.Int())

	cli := initializeDockerClient(hash)

	assert.Equal(t, cli.runtimeHash, hash)
	assert.Assert(t, cli.tree != nil)
	assert.Assert(t, is.Nil(cli.dc))
}

func TestDockerInitialization(t *testing.T) {
	// Generate a random hash for testing
	hash := strconv.Itoa(rand.Int())

	cli := initializeDockerClient(hash)
	cli.Initialize()

	assert.Assert(t, cli.dc != nil)
	//assert.Assert(t, is.Nil(cli.dc.GetContext()))
	assert.Assert(t, cli.dc.GetDockerClient() != nil)
	assert.Assert(t, cli.dc.GetNetwork().ID != "")
}
