package history

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

// PathFor is where a file kept between runs lives: home/.trimloop/base+ext
// for Claude Code's default folder, or base-<id>+ext for another
// CLAUDE_CONFIG_DIR, so runs over different folders (often different
// accounts) do not mix their figures. The id is a hash of the folder's path.
func PathFor(home, configDir, base, ext string) string {
	dir := filepath.Join(home, ".trimloop")
	if abs, err := filepath.Abs(configDir); configDir != "" && err == nil {
		configDir = abs
	}
	if configDir == "" || filepath.Clean(configDir) == filepath.Join(home, ".claude") {
		return filepath.Join(dir, base+ext) // the default folder, however named
	}
	sum := sha256.Sum256([]byte(filepath.Clean(configDir)))
	return filepath.Join(dir, fmt.Sprintf("%s-%x%s", base, sum[:4], ext))
}
