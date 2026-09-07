//go:build !js

package pretty

// defaultOutputPaths is where a line goes when nothing said.
//
// A process has a standard error and this is what it is for.
func defaultOutputPaths() []string { return []string{"stderr"} }
