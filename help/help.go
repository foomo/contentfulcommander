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

help [command] - Display this help screen or the 'command' specific one
chid - Change the Sys.ID of an entry
modeldiff - Compare two content models across spaces and environments
`)
		os.Exit(0)
	}
	switch args[0] {
	case "chid":
		fmt.Println(`
usage: contentfulcommander space chid oldid newid

Makes a copy of the entry with ID equal to 'newid'. Restores all references and preserves the publishing status.
The 'oldid' version of the entry is archived unless 'deleteold' is passed. 
The 'space' parameter is specified in the form spaceid[/environment].
`)
	case "modeldiff":
		fmt.Println(`
usage: contentfulcommander modeldiff firstspace secondspace

Compares the content model of two spaces and shows the differences. The 'firstspace' and 'secondspace' 
parameters are specified in the form spaceid[/environment]. 
`)
	}
}
