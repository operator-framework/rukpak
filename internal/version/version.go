package version

import (
	"fmt"
	"runtime/debug"
)

// GitCommit indicates which commit the rukpak binaries were built from
var (
	gitCommit  string = "unknown"
	commitDate string = "unknown"
	repoState  string = "unknown"
)

func String() string {
	return fmt.Sprintf("revision: %q, date: %q, state: %q", gitCommit, commitDate, repoState)
}

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			gitCommit = setting.Value
		case "vcs.time":
			commitDate = setting.Value
		case "vcs.modified":
			switch setting.Value {
			case "false":
				repoState = "clean"
			case "true":
				repoState = "dirty"
			default:
				repoState = "unknown"
			}
		}
	}
}
