// Package sqlutil holds small SQL helpers shared by the DAO/service layers.
package sqlutil

import "strings"

// LikeEscape is the LIKE escape character used with EscapeLike. '!' is used
// instead of a backslash because a backslash needs different quoting in
// MySQL (where it is also the default string escape) and SQLite; every query
// using EscapeLike must say `LIKE ? ESCAPE '!'`, which both engines accept.
const LikeEscape = "!"

var likeReplacer = strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`)

// EscapeLike escapes the LIKE wildcards % and _ and the escape character
// itself so s matches literally. Use with `col LIKE ? ESCAPE '!'`.
func EscapeLike(s string) string { return likeReplacer.Replace(s) }

// Contains returns a `%<escaped s>%` pattern for a substring search with
// `col LIKE ? ESCAPE '!'`.
func Contains(s string) string { return "%" + EscapeLike(s) + "%" }
