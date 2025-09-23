package tui

import (
	"fmt"
	"strings"
)

// Color represents an ANSI color
type Color int

// ANSI color constants
const (
	ColorDefault Color = iota
	ColorBlack
	ColorRed
	ColorGreen
	ColorYellow
	ColorBlue
	ColorMagenta
	ColorCyan
	ColorWhite
	ColorBrightBlack
	ColorBrightRed
	ColorBrightGreen
	ColorBrightYellow
	ColorBrightBlue
	ColorBrightMagenta
	ColorBrightCyan
	ColorBrightWhite
)

// Style represents text styling attributes
type Style struct {
	Foreground Color
	Background Color
	Bold       bool
	Italic     bool
	Underline  bool
	Strikethrough bool
	Blink      bool
	Reverse    bool
	Hidden     bool
	Dim        bool
}

// NewStyle creates a new style with default values
func NewStyle() Style {
	return Style{
		Foreground: ColorDefault,
		Background: ColorDefault,
	}
}

// WithForeground sets the foreground color
func (s Style) WithForeground(c Color) Style {
	s.Foreground = c
	return s
}

// WithBackground sets the background color
func (s Style) WithBackground(c Color) Style {
	s.Background = c
	return s
}

// WithBold enables bold text
func (s Style) WithBold() Style {
	s.Bold = true
	return s
}

// WithItalic enables italic text
func (s Style) WithItalic() Style {
	s.Italic = true
	return s
}

// WithUnderline enables underlined text
func (s Style) WithUnderline() Style {
	s.Underline = true
	return s
}

// WithStrikethrough enables strikethrough text
func (s Style) WithStrikethrough() Style {
	s.Strikethrough = true
	return s
}

// WithBlink enables blinking text
func (s Style) WithBlink() Style {
	s.Blink = true
	return s
}

// WithReverse enables reverse video
func (s Style) WithReverse() Style {
	s.Reverse = true
	return s
}

// WithDim enables dim/faint text
func (s Style) WithDim() Style {
	s.Dim = true
	return s
}

// String returns the ANSI escape sequence for this style
func (s Style) String() string {
	var codes []string

	// Reset
	codes = append(codes, "0")

	// Attributes
	if s.Bold {
		codes = append(codes, "1")
	}
	if s.Dim {
		codes = append(codes, "2")
	}
	if s.Italic {
		codes = append(codes, "3")
	}
	if s.Underline {
		codes = append(codes, "4")
	}
	if s.Blink {
		codes = append(codes, "5")
	}
	if s.Reverse {
		codes = append(codes, "7")
	}
	if s.Hidden {
		codes = append(codes, "8")
	}
	if s.Strikethrough {
		codes = append(codes, "9")
	}

	// Foreground color
	if s.Foreground != ColorDefault {
		codes = append(codes, s.Foreground.foregroundCode())
	}

	// Background color
	if s.Background != ColorDefault {
		codes = append(codes, s.Background.backgroundCode())
	}

	return fmt.Sprintf("\033[%sm", strings.Join(codes, ";"))
}

// Apply applies the style to the given text
func (s Style) Apply(text string) string {
	if s.IsEmpty() {
		return text
	}
	return s.String() + text + "\033[0m"
}

// IsEmpty checks if the style has no attributes set
func (s Style) IsEmpty() bool {
	return s == NewStyle()
}

// foregroundCode returns the ANSI code for foreground color
func (c Color) foregroundCode() string {
	switch c {
	case ColorBlack:
		return "30"
	case ColorRed:
		return "31"
	case ColorGreen:
		return "32"
	case ColorYellow:
		return "33"
	case ColorBlue:
		return "34"
	case ColorMagenta:
		return "35"
	case ColorCyan:
		return "36"
	case ColorWhite:
		return "37"
	case ColorBrightBlack:
		return "90"
	case ColorBrightRed:
		return "91"
	case ColorBrightGreen:
		return "92"
	case ColorBrightYellow:
		return "93"
	case ColorBrightBlue:
		return "94"
	case ColorBrightMagenta:
		return "95"
	case ColorBrightCyan:
		return "96"
	case ColorBrightWhite:
		return "97"
	default:
		return "39"
	}
}

// backgroundCode returns the ANSI code for background color
func (c Color) backgroundCode() string {
	switch c {
	case ColorBlack:
		return "40"
	case ColorRed:
		return "41"
	case ColorGreen:
		return "42"
	case ColorYellow:
		return "43"
	case ColorBlue:
		return "44"
	case ColorMagenta:
		return "45"
	case ColorCyan:
		return "46"
	case ColorWhite:
		return "47"
	case ColorBrightBlack:
		return "100"
	case ColorBrightRed:
		return "101"
	case ColorBrightGreen:
		return "102"
	case ColorBrightYellow:
		return "103"
	case ColorBrightBlue:
		return "104"
	case ColorBrightMagenta:
		return "105"
	case ColorBrightCyan:
		return "106"
	case ColorBrightWhite:
		return "107"
	default:
		return "49"
	}
}

// RGB represents an RGB color
type RGB struct {
	R, G, B uint8
}

// NewRGB creates a new RGB color
func NewRGB(r, g, b uint8) RGB {
	return RGB{R: r, G: g, B: b}
}

// Foreground returns the ANSI escape sequence for RGB foreground
func (rgb RGB) Foreground() string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", rgb.R, rgb.G, rgb.B)
}

// Background returns the ANSI escape sequence for RGB background
func (rgb RGB) Background() string {
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", rgb.R, rgb.G, rgb.B)
}

// Apply applies RGB color to text
func (rgb RGB) Apply(text string, background bool) string {
	if background {
		return rgb.Background() + text + "\033[0m"
	}
	return rgb.Foreground() + text + "\033[0m"
}

// Gradient creates a gradient between two RGB colors
func Gradient(start, end RGB, steps int) []RGB {
	if steps <= 1 {
		return []RGB{start}
	}

	colors := make([]RGB, steps)
	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps-1)
		colors[i] = RGB{
			R: uint8(float64(start.R) + t*float64(end.R-start.R)),
			G: uint8(float64(start.G) + t*float64(end.G-start.G)),
			B: uint8(float64(start.B) + t*float64(end.B-start.B)),
		}
	}
	return colors
}

// RainbowGradient creates a rainbow gradient with the full spectrum
func RainbowGradient(steps int) []RGB {
	if steps <= 1 {
		return []RGB{NewRGB(255, 0, 0)}
	}

	// Use the classic rainbow color stops for a proper spectrum
	rainbowStops := []RGB{
		NewRGB(255, 0, 0),     // Red
		NewRGB(255, 127, 0),   // Orange
		NewRGB(255, 255, 0),   // Yellow
		NewRGB(0, 255, 0),     // Green
		NewRGB(0, 0, 255),     // Blue
		NewRGB(75, 0, 130),    // Indigo
		NewRGB(148, 0, 211),   // Violet
	}

	colors := make([]RGB, steps)

	for i := 0; i < steps; i++ {
		// Calculate position in the rainbow (0.0 to 1.0)
		t := float64(i) / float64(steps-1)
		// Scale to the number of segments (6 transitions between 7 colors)
		position := t * 6.0
		segment := int(position)
		localT := position - float64(segment)

		if segment >= 6 {
			colors[i] = rainbowStops[6]
		} else {
			start := rainbowStops[segment]
			end := rainbowStops[segment+1]
			colors[i] = RGB{
				R: uint8(float64(start.R)*(1-localT) + float64(end.R)*localT),
				G: uint8(float64(start.G)*(1-localT) + float64(end.G)*localT),
				B: uint8(float64(start.B)*(1-localT) + float64(end.B)*localT),
			}
		}
	}

	return colors
}

// SmoothRainbow creates a smooth rainbow using proper HSL conversion
func SmoothRainbow(steps int) []RGB {
	if steps <= 1 {
		return []RGB{NewRGB(255, 0, 0)}
	}

	colors := make([]RGB, steps)
	for i := 0; i < steps; i++ {
		hue := float64(i) / float64(steps) * 360.0
		colors[i] = hslToRGB(hue, 1.0, 0.5)
	}
	return colors
}

// MultiGradient creates a gradient through multiple color stops
func MultiGradient(stops []RGB, steps int) []RGB {
	if len(stops) == 0 {
		return []RGB{}
	}
	if len(stops) == 1 {
		result := make([]RGB, steps)
		for i := range result {
			result[i] = stops[0]
		}
		return result
	}

	colors := make([]RGB, steps)

	for i := 0; i < steps; i++ {
		position := float64(i) / float64(steps-1) * float64(len(stops)-1)
		segment := int(position)
		if segment >= len(stops)-1 {
			colors[i] = stops[len(stops)-1]
		} else {
			localT := position - float64(segment)
			start := stops[segment]
			end := stops[segment+1]
			colors[i] = RGB{
				R: uint8(float64(start.R)*(1-localT) + float64(end.R)*localT),
				G: uint8(float64(start.G)*(1-localT) + float64(end.G)*localT),
				B: uint8(float64(start.B)*(1-localT) + float64(end.B)*localT),
			}
		}
	}
	return colors
}

// hslToRGB converts HSL color space to RGB
func hslToRGB(h, s, l float64) RGB {
	// Normalize hue to 0-1 range
	h = h / 360.0

	var r, g, b float64

	if s == 0 {
		// Grayscale
		r, g, b = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q

		r = hueToRGB(p, q, h+1.0/3.0)
		g = hueToRGB(p, q, h)
		b = hueToRGB(p, q, h-1.0/3.0)
	}

	return RGB{
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
	}
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}