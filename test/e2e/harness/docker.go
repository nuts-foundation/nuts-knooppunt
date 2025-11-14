package harness

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

func createDockerNetwork(t *testing.T) (*testcontainers.DockerNetwork, error) {
	dockerNetwork, err := network.New(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := dockerNetwork.Remove(context.Background()); err != nil {
			panic(err)
		}
	})
	return dockerNetwork, err
}
