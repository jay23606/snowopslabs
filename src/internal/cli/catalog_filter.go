// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
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

// noMatch is the line printed when a filter leaves nothing to list.
func noMatch(noun string, f schema.Filter) string {
	var parts []string
	if f.Track != "" {
		parts = append(parts, "track "+f.Track)
	}
	if f.Level != "" {
		parts = append(parts, "level "+f.Level)
	}
	return fmt.Sprintf("No %s match %s. Drop a filter to see more.", noun, strings.Join(parts, " and "))
}

// minutesCell renders a duration column: the minutes, or "-" when unstated.
func minutesCell(m int) string {
	if m <= 0 {
		return "-"
	}
	return strconv.Itoa(m)
}
