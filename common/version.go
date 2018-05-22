package common

import "runtime"

var (
	version   = "unknown"
	gitcommit = "unknown"
	buildtime = "unknown"
)

type VersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitcommit"`
	BuildTime string `json:"buildtime"`
	GoVersion string `json:"goversion"`
	OSArch    string `json:"os/arch"`
}

func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version:   version,
		GitCommit: gitcommit,
		BuildTime: buildtime,
		GoVersion: runtime.Version(),
		OSArch:    runtime.GOOS + "/" + runtime.GOARCH,
	}
}
