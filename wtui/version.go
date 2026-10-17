package main

import "runtime/debug"

// devVersion is what a build with no version baked in reports.
const devVersion = "dev"

// version is the release wtui was built from, baked in at build time with
// -ldflags "-X main.version=v0.1.0".
var version = devVersion

// currentVersion is what `wtui --version` prints.
func currentVersion() string {
	info, _ := debug.ReadBuildInfo()
	return resolveVersion(version, info)
}

// resolveVersion returns the baked-in version if there is one. Otherwise it
// falls back to the module version Go records for `go install …@version`
// builds; local builds record "(devel)" and report devVersion.
func resolveVersion(baked string, info *debug.BuildInfo) string {
	if baked != devVersion || info == nil {
		return baked
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return devVersion
}
