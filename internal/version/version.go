package version

import (
	"fmt"
	"runtime/debug"
)

var (
	gitCommit  = "unknown"
	commitDate = "unknown"
	repoState  = "unknown"

	stateMap = map[string]string{
		"true":  "dirty",
		"false": "clean",
	}
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
			if v, ok := stateMap[setting.Value]; ok {
				repoState = v
			}
		}
	}
}
