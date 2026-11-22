// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	schema "github.com/snowops-oss/snowopslabs/pkg/scenario"
)

// catalogFlags holds the --track and --level values of a catalog list command.
type catalogFlags struct{ track, level string }

// add registers --track and --level on cmd, naming the valid values in help.
func (c *catalogFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringVar(&c.track, "track", "", "only this track: "+strings.Join(schema.Tracks, ", "))
	cmd.Flags().StringVar(&c.level, "level", "", "only this level: "+strings.Join(schema.Levels, ", "))
}

// filter validates the flag values, so a typo names the valid ones instead of
// printing an empty list.
func (c *catalogFlags) filter() (schema.Filter, error) {
	return schema.ParseFilter(c.track, c.level)
}

// noMatch is the line printed when a filter leaves nothing to list. It names
// the track and level as a learner knows them.
func noMatch(noun string, f schema.Filter) string {
	var where []string
	if f.Track != "" {
		where = append(where, "on the "+schema.TrackLabel(f.Track)+" track")
	}
	if f.Level != "" {
		where = append(where, "at "+schema.LevelLabel(f.Level)+" level")
	}
	return fmt.Sprintf("No %s %s yet. Drop a filter to see more.", noun, strings.Join(where, " "))
}

// printMeta writes the Track, Level and Time lines of an info command. Time is
// left out when the item states no minutes.
func printMeta(w io.Writer, m schema.Meta) {
	m.ApplyDefaults()
	fmt.Fprintf(w, "Track:       %s\n", schema.TrackLabel(m.Track))
	fmt.Fprintf(w, "Level:       %s\n", schema.LevelLabel(m.Level))
	if m.Minutes > 0 {
		fmt.Fprintf(w, "Time:        ~%d min\n", m.Minutes)
	}
}

// minutesCell renders a duration column: the minutes, or "-" when unstated.
func minutesCell(m int) string {
	if m <= 0 {
		return "-"
	}
	return strconv.Itoa(m)
}

// warnLoadErrors tells the reader which items a list left out because they
// failed to load, one line each with the reason and the fix. It writes
// nothing when every item loaded.
func warnLoadErrors(w io.Writer, kind string, errs map[string]error) {
	for _, key := range slices.Sorted(maps.Keys(errs)) {
		fmt.Fprintf(w, "Warning: %s %s failed to load and is not listed: %v\n", kind, key, errs[key])
	}
}
