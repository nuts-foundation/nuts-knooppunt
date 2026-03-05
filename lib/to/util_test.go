package to

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONMap(t *testing.T) {
	t.Run("struct with simple fields", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		person := Person{Name: "John", Age: 30}

		result, err := JSONMap(person)

		require.NoError(t, err)
		assert.Equal(t, "John", result["name"])
		assert.Equal(t, float64(30), result["age"])
	})
}
