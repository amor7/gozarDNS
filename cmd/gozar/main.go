// Command gozar is the whole of Gozar: the hub, the gate, the setup wizard
// and the tools, in one binary.
package main

import (
	"os"

	"github.com/amor7/gozarDNS/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
