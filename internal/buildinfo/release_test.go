package buildinfo

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
The installer says the version the binaries inside it say.

The binaries are stamped from VERSION by the Makefile. The installer's version
is app.version in fynstall.yaml, which fynstall reads as a literal: it names the
output files, and it is what a later upgrade compares. Two files and one number,
so a release that bumps VERSION alone ships an installer that calls itself the
release before (spec 004 R5).
*/
func TestTheInstallerCarriesTheReleaseVersion(t *testing.T) {
	version, err := os.ReadFile("../../VERSION")
	require.NoError(t, err)
	want := strings.TrimSpace(string(version))
	require.NotEmpty(t, want, "VERSION holds no version")

	manifest, err := os.ReadFile("../../fynstall.yaml")
	require.NoError(t, err)
	// The one `version:` key in the file is app's; a second would be a new
	// fynstall key this test has to learn about, and it says so.
	found := regexp.MustCompile(`(?m)^\s+version:\s*(\S+)`).FindAllStringSubmatch(string(manifest), -1)
	require.Len(t, found, 1, "fynstall.yaml should hold exactly one version key")
	require.Equal(t, want, found[0][1], "fynstall.yaml's app.version is not VERSION's")
}
