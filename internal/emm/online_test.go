package emm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/enshroudedbonker/internal/wine"
)

/*
TestGet_RealReleaseRunsUnderRealWine is spec 003's integration check: the
pinned release downloaded from GitHub matches the pin, and runs under the
Wine this machine would pick. It needs the network and Wine, so it runs only
with ENSHROUDEDBONKER_ONLINE=1. ENSHROUDEDBONKER_COMPATDATA, the game's
compatdata/1203620 directory, picks its Proton when set; otherwise the
system's wine is used.
*/
func TestGet_RealReleaseRunsUnderRealWine(t *testing.T) {
	if os.Getenv("ENSHROUDEDBONKER_ONLINE") != "1" {
		t.Skip("set ENSHROUDEDBONKER_ONLINE=1 to download the real release")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	tools := t.TempDir()
	p, err := Get(ctx, GetOptions{ToolsDir: tools})
	require.NoError(t, err)
	require.NoError(t, Verify(p))

	compat := os.Getenv("ENSHROUDEDBONKER_COMPATDATA")
	r, _, err := Locate(tools, "", func() (wine.Wine, error) { return wine.Find(compat) })
	if r.Wine == nil && err != nil {
		t.Skipf("no Wine on this machine: %v", err)
	}
	require.NoError(t, err)
	v, err := Version(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, "mod-manager-cli "+Tag[1:], v)
	if r.Wine != nil {
		_ = r.Wine.Kill(r.Prefix)
	}
}
