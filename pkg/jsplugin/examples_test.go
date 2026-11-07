package jsplugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExamplePluginsCompile loads every worked example under examples/plugins
// through the same compiler the host uses for uploaded and built-in plugins. An
// example is published as copyable reference code, so a contract change that
// makes one fail to load must fail here instead of reaching plugin authors.
func TestExamplePluginsCompile(t *testing.T) {
	directories, err := filepath.Glob(filepath.Join("..", "..", "examples", "plugins", "*", "plugin.js"))
	require.NoError(t, err)
	require.NotEmpty(t, directories, "no example plugins found under examples/plugins/*/plugin.js")

	// One gateway installs every example, so the keys must stay distinct.
	keys := make(map[string]string, len(directories))
	for _, path := range directories {
		directory := filepath.Dir(path)
		t.Run(filepath.Base(directory), func(t *testing.T) {
			source, readErr := os.ReadFile(path)
			require.NoError(t, readErr)

			plugin, compileErr := CompilePlugin(string(source), Options{})
			require.NoError(t, compileErr)
			require.NotEmpty(t, plugin.Meta.Key)

			if other, duplicate := keys[plugin.Meta.Key]; duplicate {
				t.Fatalf("example key %q is also declared by %s", plugin.Meta.Key, other)
			}
			keys[plugin.Meta.Key] = path
		})
	}
	assert.Len(t, keys, len(directories))
}
