// Package buildinfo carries the version identity of the binary.
//
// Both binaries (the Wails app and kubby-cli) read from here, so "which version
// am I running" has exactly one answer. Without this, a bug report is
// unanswerable — there is no way to tell which build produced it.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Version is the release version. It is the source of truth when built plainly;
// a release build overrides it:
//
//	go build -ldflags "-X kubby/internal/buildinfo.Version=1.2.0" ./...
//
// Keep the `-dev` suffix on the checked-in value so an unreleased build is never
// mistaken for a release.
var Version = "0.2.0-dev"

// Commit and Date are optional overrides for the same reason. When the source is
// a git checkout, Go fills the equivalent information into the build info
// automatically and vcs() below picks it up — so these only need setting for
// builds made outside a repository (a CI tarball, for instance).
var (
	Commit = ""
	Date   = ""
)

// Info is the full identity of this binary.
type Info struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`   // short revision, "-dirty" when the tree was modified
	Date     string `json:"date"`     // build or commit time
	Go       string `json:"go"`       // toolchain version
	Platform string `json:"platform"` // GOOS/GOARCH
}

// Get returns this binary's identity, filling commit and date from Go's embedded
// VCS stamps when they were not set at link time.
func Get() Info {
	commit, date := Commit, Date
	if commit == "" || date == "" {
		vcsCommit, vcsDate := vcs()
		if commit == "" {
			commit = vcsCommit
		}
		if date == "" {
			date = vcsDate
		}
	}
	return Info{
		Version:  Version,
		Commit:   commit,
		Date:     date,
		Go:       runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
}

// String renders the identity on one line, e.g.
// "Kubby 0.2.0-dev (a1b2c3d-dirty, 2026-08-19) go1.26.6 windows/amd64".
func (i Info) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Kubby %s", i.Version)
	switch {
	case i.Commit != "" && i.Date != "":
		fmt.Fprintf(&b, " (%s, %s)", i.Commit, i.Date)
	case i.Commit != "":
		fmt.Fprintf(&b, " (%s)", i.Commit)
	}
	fmt.Fprintf(&b, " %s %s", i.Go, i.Platform)
	return b.String()
}

// vcs reads the revision Go embeds when building from a version-controlled
// directory. Returns empty strings when the source is not a checkout — which is
// the case for this repository today, so the About box says so rather than
// showing a blank field.
func vcs() (commit, date string) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", ""
	}
	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
			if len(commit) > 7 {
				commit = commit[:7]
			}
		case "vcs.time":
			date = s.Value
			if len(date) >= 10 {
				date = date[:10] // the day is enough; the commit identifies the rest
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if dirty && commit != "" {
		commit += "-dirty"
	}
	return commit, date
}
