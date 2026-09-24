package sqlutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"":          "",
		"abc":       "abc",
		"50%":       "50!%",
		"a_b":       "a!_b",
		"x!y":       "x!!y",
		"!%_":       "!!!%!_",
		`back\sl`:   `back\sl`,
		"中文_%!":     "中文!_!%!!",
		"'; drop--": "'; drop--",
	}
	for in, want := range cases {
		require.Equal(t, want, EscapeLike(in), in)
	}
	require.Equal(t, "%a!_b%", Contains("a_b"))
}

type row struct {
	ID   int
	Name string
}

// The ESCAPE clause behaves as documented on SQLite (MySQL uses the same
// standard syntax); wildcards in user input only match literally.
func TestContains_SQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&row{}))
	for i, n := range []string{"alpha", "a_b", "axb", "100%", "1000", "x!y", "xy", `c\d`} {
		require.NoError(t, db.Create(&row{ID: i + 1, Name: n}).Error)
	}
	find := func(kw string) []string {
		var rs []row
		require.NoError(t, db.Where("name LIKE ? ESCAPE '!'", Contains(kw)).Order("id").Find(&rs).Error)
		out := []string{}
		for _, r := range rs {
			out = append(out, r.Name)
		}
		return out
	}
	require.Equal(t, []string{"a_b"}, find("_"))
	require.Equal(t, []string{"a_b"}, find("a_b"))
	require.Equal(t, []string{"100%"}, find("%"))
	require.Equal(t, []string{"x!y"}, find("!"))
	require.Equal(t, []string{`c\d`}, find(`\`))
	require.Equal(t, []string{"alpha"}, find("lph"))
	require.Len(t, find(""), 8)
}
