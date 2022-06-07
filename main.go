package main

import (
	"flag"
	"log"
	"os"

	"github.com/foomo/contentfulcommander/cmd/chid"
	"github.com/foomo/contentfulcommander/contentfulclient"
	"github.com/foomo/contentfulcommander/help"
)

var VERSION = "v0.0.1"

func main() {
	cmaKey := contentfulclient.GetCmaKeyFromRcFile()
	if cmaKey == "" {
		help.FatalNoCMAKey()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		help.GetHelp(nil)
		os.Exit(0)
	}
	command := args[0]
	params := args[1:]
	err := runCommand(cmaKey, command, params)
	if err != nil {
		log.Fatal(err)
	}

}

func ensureExtraParams(command string, params []string, size int) {
	if len(params) != size {
		log.Printf("You need to pass %d parameters to this command but I got %d\n", size, len(params))
		help.GetHelp([]string{command})
		os.Exit(1)
	}
}

func runCommand(cmaKey, command string, params []string) error {
	switch command {
	case "help":
		help.GetHelp(params)
		os.Exit(0)
	case "version":
		log.Println(VERSION)
		os.Exit(0)
	default:
		client := contentfulclient.GetCMA(cmaKey)
		switch command {
		case "chid":
			ensureExtraParams(command, params, 3)
			return chid.Run(client, params)
		}
		os.Exit(0)
	}
	return nil
}
