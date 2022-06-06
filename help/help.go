package help

import (
	"fmt"
	"os"
)

func FatalNoCMAKey() {
	fmt.Println(`
error: you need to be logged in to Contentful to use contentfulcommander

1) Install the Contentful CLI, see https://www.contentful.com/developers/docs/tutorials/cli/installation/
2) Log in to Contentful from a terminal with:
	contenful login
`)
	os.Exit(1)
}

func GetHelp(args []string) {
	if len(args) == 0 {
		fmt.Println(`
usage: contentfulcommander command [params]

Supported values for 'command' are:

help [command] - Displays this help screen or the 'command' specific one
chid - Changes the Sys.ID of an entry
`)
		os.Exit(0)
	}
	switch args[0] {
	case "chid":
		fmt.Println(`
usage: contentfulcommander chid oldid newid

Makes a copy of the entry with ID equal to 'newid'. Restores all references and preserves the publishing status.
The 'oldid' version of the entry is archived unless 'deleteold' is passed.
`)
	}
}
