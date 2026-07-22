package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envLookup(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

func TestLoad_FromFlags(t *testing.T) {
	cfg, err := Load(
		[]string{"--host=localhost", "--port=5432", "--user=alice", "--password=secret", "--dbname=app"},
		envLookup(nil),
	)

	require.NoError(t, err)
	assert.Equal(t, Config{
		Host: "localhost", Port: "5432", User: "alice",
		Password: "secret", Database: "app", SSLMode: "prefer", Lang: "en",
	}, cfg)
}

func TestLoad_FromEnv(t *testing.T) {
	env := map[string]string{
		"PGHOST": "db.internal", "PGPORT": "6543", "PGUSER": "bob",
		"PGPASSWORD": "hunter2", "PGDATABASE": "billing", "PGSSLMODE": "require",
	}

	cfg, err := Load(nil, envLookup(env))

	require.NoError(t, err)
	assert.Equal(t, Config{
		Host: "db.internal", Port: "6543", User: "bob",
		Password: "hunter2", Database: "billing", SSLMode: "require", Lang: "en",
	}, cfg)
}

func TestLoad_FlagsOverrideEnv(t *testing.T) {
	env := map[string]string{"PGHOST": "env-host", "PGPORT": "1", "PGUSER": "env-user", "PGDATABASE": "env-db"}

	cfg, err := Load([]string{"--host=flag-host"}, envLookup(env))

	require.NoError(t, err)
	assert.Equal(t, "flag-host", cfg.Host)
	assert.Equal(t, "1", cfg.Port) // still from env
}

func TestLoad_DefaultsSSLModeToPrefer(t *testing.T) {
	cfg, err := Load(
		[]string{"--host=h", "--port=5432", "--user=u", "--dbname=d"},
		envLookup(nil),
	)

	require.NoError(t, err)
	assert.Equal(t, "prefer", cfg.SSLMode)
}

func TestLoad_DefaultsLangToEnglish(t *testing.T) {
	cfg, err := Load(
		[]string{"--host=h", "--port=5432", "--user=u", "--dbname=d"},
		envLookup(nil),
	)

	require.NoError(t, err)
	assert.Equal(t, "en", cfg.Lang)
}

func TestLoad_LangFromFlag(t *testing.T) {
	cfg, err := Load(
		[]string{"--host=h", "--port=5432", "--user=u", "--dbname=d", "--lang=fr"},
		envLookup(nil),
	)

	require.NoError(t, err)
	assert.Equal(t, "fr", cfg.Lang)
}

func TestLoad_LangFromEnv(t *testing.T) {
	env := map[string]string{"PGHOST": "h", "PGPORT": "5432", "PGUSER": "u", "PGDATABASE": "d", "PGTUI_LANG": "fr"}

	cfg, err := Load(nil, envLookup(env))

	require.NoError(t, err)
	assert.Equal(t, "fr", cfg.Lang)
}

func TestLoad_LangFlagOverridesEnv(t *testing.T) {
	env := map[string]string{"PGHOST": "h", "PGPORT": "5432", "PGUSER": "u", "PGDATABASE": "d", "PGTUI_LANG": "fr"}

	cfg, err := Load([]string{"--lang=de"}, envLookup(env))

	require.NoError(t, err)
	assert.Equal(t, "de", cfg.Lang)
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	_, err := Load(nil, envLookup(nil))

	require.Error(t, err)
	var missingErr *MissingFieldsError
	require.ErrorAs(t, err, &missingErr)
	assert.ElementsMatch(t, []string{
		"host (--host or PGHOST)",
		"port (--port or PGPORT)",
		"user (--user or PGUSER)",
		"dbname (--dbname or PGDATABASE)",
	}, missingErr.Fields)
}

func TestLoad_MissingPasswordIsNotAnError(t *testing.T) {
	cfg, err := Load(
		[]string{"--host=h", "--port=5432", "--user=u", "--dbname=d"},
		envLookup(nil),
	)

	require.NoError(t, err)
	assert.Empty(t, cfg.Password)
}

func TestConfig_DSN(t *testing.T) {
	cfg := Config{Host: "localhost", Port: "5432", User: "alice", Password: "s3cret", Database: "app", SSLMode: "disable"}

	assert.Equal(t, "postgres://alice:s3cret@localhost:5432/app?sslmode=disable", cfg.DSN())
}

func TestResolvePassword_UsesExistingPassword(t *testing.T) {
	cfg := Config{Password: "already-set"}
	promptCalled := false

	got, err := ResolvePassword(cfg, func() (string, error) {
		promptCalled = true
		return "should not be used", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "already-set", got.Password)
	assert.False(t, promptCalled, "should not prompt when a password was already supplied")
}

func TestResolvePassword_PromptsWhenMissing(t *testing.T) {
	cfg := Config{}

	got, err := ResolvePassword(cfg, func() (string, error) {
		return "typed-at-prompt", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "typed-at-prompt", got.Password)
}

func TestResolvePassword_PropagatesPromptError(t *testing.T) {
	cfg := Config{}

	_, err := ResolvePassword(cfg, func() (string, error) {
		return "", errors.New("no tty")
	})

	assert.EqualError(t, err, "no tty")
}
