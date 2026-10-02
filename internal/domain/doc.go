// Package domain is what wtr knows about watching files and running a command
// on them, with no disk, process or terminal in it: which files a watchlist
// names and which it leaves out, how one look at them differs from the last,
// when a change calls for a run, what the command is for the files, how a run
// ended, and how its output differs from the output it is compared with.
//
// It imports the standard library, and two libraries that only compute:
// doublestar for the syntax of globs, and go-udiff's lcs for diffing. Reading
// the disk with a glob is the disk package's business, not this one's.
package domain
