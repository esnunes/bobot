// skills/skills.go
package skills

import "embed"

// FS contains all embedded skill files.
// Add .md files to this directory to include them as skills.
//
//go:embed *.md
var FS embed.FS
