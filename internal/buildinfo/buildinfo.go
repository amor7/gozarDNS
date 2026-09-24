// Package buildinfo carries values stamped in at link time.
package buildinfo

// Version is set with -ldflags "-X github.com/amor7/gozar/internal/buildinfo.Version=v1.2.3".
var Version = "dev"

// Repo is where releases are published; `gozar update` reads from here.
const Repo = "amor7/gozar"
