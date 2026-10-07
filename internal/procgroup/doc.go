// Package procgroup makes CommandContext cancellation end a shell command's
// whole process tree. Without it, a descendant that inherited the command's
// output pipe keeps running after the shell is killed, so a reader of that
// pipe never sees EOF.
package procgroup
