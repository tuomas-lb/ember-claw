package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoad_BasicKeyValue(t *testing.T) {
	path := writeEnvFile(t, "FOO=bar\nBAZ=qux\n")

	os.Unsetenv("FOO")
	os.Unsetenv("BAZ")
	t.Cleanup(func() { os.Unsetenv("FOO"); os.Unsetenv("BAZ") })

	require.NoError(t, Load(path))
	assert.Equal(t, "bar", os.Getenv("FOO"))
	assert.Equal(t, "qux", os.Getenv("BAZ"))
}

func TestLoad_SkipsComments(t *testing.T) {
	path := writeEnvFile(t, "# comment\nKEY1=val1\n# another\nKEY2=val2\n")

	os.Unsetenv("KEY1")
	os.Unsetenv("KEY2")
	t.Cleanup(func() { os.Unsetenv("KEY1"); os.Unsetenv("KEY2") })

	require.NoError(t, Load(path))
	assert.Equal(t, "val1", os.Getenv("KEY1"))
	assert.Equal(t, "val2", os.Getenv("KEY2"))
}

func TestLoad_SkipsEmptyLines(t *testing.T) {
	path := writeEnvFile(t, "\n\nEMPTY_TEST=yes\n\n")

	os.Unsetenv("EMPTY_TEST")
	t.Cleanup(func() { os.Unsetenv("EMPTY_TEST") })

	require.NoError(t, Load(path))
	assert.Equal(t, "yes", os.Getenv("EMPTY_TEST"))
}

func TestLoad_DoubleQuotedValues(t *testing.T) {
	path := writeEnvFile(t, `QUOTED="hello world"`)

	os.Unsetenv("QUOTED")
	t.Cleanup(func() { os.Unsetenv("QUOTED") })

	require.NoError(t, Load(path))
	assert.Equal(t, "hello world", os.Getenv("QUOTED"))
}

func TestLoad_SingleQuotedValues(t *testing.T) {
	path := writeEnvFile(t, `SQUOTED='single quoted'`)

	os.Unsetenv("SQUOTED")
	t.Cleanup(func() { os.Unsetenv("SQUOTED") })

	require.NoError(t, Load(path))
	assert.Equal(t, "single quoted", os.Getenv("SQUOTED"))
}

func TestLoad_DoesNotOverrideExisting(t *testing.T) {
	path := writeEnvFile(t, "EXISTING_VAR=new_value\n")

	os.Setenv("EXISTING_VAR", "old_value")
	t.Cleanup(func() { os.Unsetenv("EXISTING_VAR") })

	require.NoError(t, Load(path))
	assert.Equal(t, "old_value", os.Getenv("EXISTING_VAR"))
}

func TestLoad_FileNotExist(t *testing.T) {
	err := Load("/nonexistent/path/.env")
	assert.NoError(t, err, "Load should return nil for missing files")
}

func TestLoad_MalformedLinesIgnored(t *testing.T) {
	path := writeEnvFile(t, "no_equals_sign\nGOOD=ok\njust_key_no_val\n")

	os.Unsetenv("GOOD")
	t.Cleanup(func() { os.Unsetenv("GOOD") })

	require.NoError(t, Load(path))
	assert.Equal(t, "ok", os.Getenv("GOOD"))
}

func TestLoad_EmptyValue(t *testing.T) {
	path := writeEnvFile(t, "EMPTY_VAL=\n")

	os.Unsetenv("EMPTY_VAL")
	t.Cleanup(func() { os.Unsetenv("EMPTY_VAL") })

	require.NoError(t, Load(path))
	assert.Equal(t, "", os.Getenv("EMPTY_VAL"))
}

func TestLoad_ValueWithEquals(t *testing.T) {
	path := writeEnvFile(t, "URL=https://example.com?a=1&b=2\n")

	os.Unsetenv("URL")
	t.Cleanup(func() { os.Unsetenv("URL") })

	require.NoError(t, Load(path))
	assert.Equal(t, "https://example.com?a=1&b=2", os.Getenv("URL"))
}

func TestLoad_TrimSpaces(t *testing.T) {
	path := writeEnvFile(t, "  SPACED  =  value  \n")

	os.Unsetenv("SPACED")
	t.Cleanup(func() { os.Unsetenv("SPACED") })

	require.NoError(t, Load(path))
	assert.Equal(t, "value", os.Getenv("SPACED"))
}
