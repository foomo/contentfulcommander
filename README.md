# contentfulcommander
_A toolbox of non-trivial Contentful interactions_

Contentful Commander is a golang command line utility that simplifies development
and maintenance of Contentful spaces. 

## Installation

Build your own binary or, if you trust us install it using Homebrew:

```
brew install foomo/contentfulcommander/contentfulcommander
```

## Usage

You need to be logged in to Contentful to use contentfulcommander:

1) Install the Contentful CLI, see https://www.contentful.com/developers/docs/tutorials/cli/installation/
2) Log in to Contentful from a terminal with:
```
$ contenful login
```
3) Test it
```
$ contentfulcommander version
v0.1.0
```

### Available commands

To get the list of available commands run
```
$ contentfulcommander help
```
and to get help for each specific command run
```
$ contentfulcommander help <command>
```
Currently supported commands are:
- __chid__ - _Change the Sys.ID of an entry_. This creates a copy of the existing entry, 
respecting the publishing status. The old entry is archived
- __modeldiff__ - _Compare two content models across spaces and environments_. 


