package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadYAML(t *testing.T) {
	content := `
server:
  name: "voce-test"
  host: "0.0.0.0"
  port: 8080
logging:
  level: "debug"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(content))
	require.NoError(t, err)
	_ = tmpfile.Close()

	var cfg VoceBootstrap
	err = LoadYAML(tmpfile.Name(), &cfg)
	require.NoError(t, err)

	assert.Equal(t, "voce-test", cfg.Server.Name)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "debug", cfg.Logging.Level)
}
