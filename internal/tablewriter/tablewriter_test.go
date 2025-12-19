package tablewriter

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	require.NotNil(t, w)
	require.Equal(t, &buf, w.out)
	require.Empty(t, w.headers)
	require.Empty(t, w.rows)
	require.Empty(t, w.widths)
}

func TestEmptyTable(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Render()
	require.Empty(t, buf.String())
}

func TestTableWithHeaders(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Header([]string{"Name", "Age", "City"})
	w.Render()

	expected := `+------+-----+------+
| Name | Age | City |
+------+-----+------+
+------+-----+------+
`
	require.Equal(t, expected, buf.String())
}

func TestTableWithHeadersAndRows(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Header([]string{"Name", "Age", "City"})
	w.Append([]string{"Alice", "30", "New York"})
	w.Append([]string{"Bob", "25", "LA"})
	w.Render()

	expected := `+-------+-----+----------+
| Name  | Age | City     |
+-------+-----+----------+
| Alice | 30  | New York |
| Bob   | 25  | LA       |
+-------+-----+----------+
`
	require.Equal(t, expected, buf.String())
}

func TestTableWithoutHeaders(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Append([]string{"Alice", "30", "New York"})
	w.Append([]string{"Bob", "25", "LA"})
	w.Render()

	expected := `+-------+----+----------+
| Alice | 30 | New York |
| Bob   | 25 | LA       |
+-------+----+----------+
`
	require.Equal(t, expected, buf.String())
}

func TestTableWithVaryingColumnCounts(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Header([]string{"Col1", "Col2", "Col3", "Col4"})
	w.Append([]string{"A", "B"})                // Only 2 columns
	w.Append([]string{"C", "D", "E", "F", "G"}) // 5 columns (extra ignored)
	w.Render()

	expected := `+------+------+------+------+
| Col1 | Col2 | Col3 | Col4 |
+------+------+------+------+
| A    | B    |      |      |
| C    | D    | E    | F    |
+------+------+------+------+
`
	require.Equal(t, expected, buf.String())
}

func TestTableWithLongContent(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Header([]string{"Short", "Very Long Header Name", "Mid"})
	w.Append([]string{"a", "b", "c"})
	w.Append([]string{"This is a long cell", "short", "medium text"})
	w.Render()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Check that the table has the expected structure
	require.Len(t, lines, 6)
	require.True(t, strings.HasPrefix(lines[0], "+"))
	require.True(t, strings.Contains(lines[1], "| Short"))
	require.True(t, strings.Contains(lines[1], "Very Long Header Name"))
	require.True(t, strings.HasPrefix(lines[2], "+"))
	require.True(t, strings.Contains(lines[3], "| a"))
	require.True(t, strings.Contains(lines[4], "This is a long cell"))
	require.True(t, strings.HasPrefix(lines[5], "+"))
}

func TestSetHeaderAlias(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	// Test with Header method
	w1 := NewWriter(&buf1)
	w1.Header([]string{"A", "B"})
	w1.Append([]string{"1", "2"})
	w1.Render()

	// Test with SetHeader method
	w2 := NewWriter(&buf2)
	w2.SetHeader([]string{"A", "B"})
	w2.Append([]string{"1", "2"})
	w2.Render()

	// Both should produce the same output
	require.Equal(t, buf1.String(), buf2.String())
}

func TestTableWithANSIColors(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)

	// Headers with colors
	w.Header([]string{"Status", "Name", "Value"})

	// Rows with ANSI color codes
	w.Append([]string{
		"\033[32m✓\033[0m",         // Green checkmark
		"\033[34mBlue Text\033[0m", // Blue text
		"100",
	})
	w.Append([]string{
		"\033[31m✗\033[0m",      // Red X
		"\033[33mYellow\033[0m", // Yellow text
		"200",
	})

	w.Render()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Verify structure
	require.Len(t, lines, 6) // borders + header + 2 rows

	// Verify the output contains our ANSI codes
	require.Contains(t, output, "\033[32m")
	require.Contains(t, output, "\033[31m")
	require.Contains(t, output, "\033[34m")
	require.Contains(t, output, "\033[33m")

	// Check that borders align properly despite ANSI codes
	// All border lines should have the same visual width
	borderLines := []string{lines[0], lines[2], lines[5]}
	firstBorderLen := len(testStripANSI(borderLines[0]))
	for _, border := range borderLines {
		strippedLen := len(testStripANSI(border))
		require.Equal(t, firstBorderLen, strippedLen, "Border lines should have consistent width when ANSI codes are stripped")
	}
}

// Helper function for tests
func testStripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return re.ReplaceAllString(s, "")
}

func TestRealWorldExample(t *testing.T) {
	var buf bytes.Buffer
	table := NewWriter(&buf)
	table.Header([]string{"Rank", "Provider", "Model", "Avg Time", "Est Cost", "Price/1M", "Tokens", "Status"})

	table.Append([]string{
		"1",
		"openai",
		"gpt-4o-mini",
		"1.23s",
		"$0.0012",
		"$0.15",
		"8000",
		"✓",
	})

	table.Append([]string{
		"2",
		"anthropic",
		"claude-3-haiku",
		"0.98s",
		"$0.0008",
		"$0.25",
		"6500",
		"✓",
	})

	table.Render()

	output := buf.String()

	// Verify the output contains our data
	require.Contains(t, output, "Rank")
	require.Contains(t, output, "Provider")
	require.Contains(t, output, "openai")
	require.Contains(t, output, "anthropic")
	require.Contains(t, output, "gpt-4o-mini")
	require.Contains(t, output, "claude-3-haiku")

	// Check structure
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 6) // Top border, header, separator, 2 rows, bottom border

	// All lines should have consistent width (note: unicode characters may affect byte count)
	// Just verify that we have the expected structure
	for _, line := range lines {
		// Each line should start with either + or |
		require.True(t, strings.HasPrefix(line, "+") || strings.HasPrefix(line, "|"))
	}
}
