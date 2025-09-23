# TUI Animation Features

The Dive TUI package now supports comprehensive animation capabilities for creating dynamic, visually appealing terminal user interfaces.

## Features Implemented

### 🎨 Multi-Line Animated Content
- **AnimatedMultiLine**: Display multiple lines of content with independent animations
- **AnimatedLayout**: Enhanced layout system with dedicated animation areas
- **Positioning**: Content areas above input, headers, and footers

### 🌈 Text Color Animations
- **RainbowAnimation**: Moving rainbow effects across text
- **PulseAnimation**: Pulsing brightness effects
- **WaveAnimation**: Wave-like color transitions
- **RGB Support**: Full 24-bit color support with smooth gradients

### 📊 Animated UI Components
- **AnimatedStatusBar**: Status bars with animated values and icons
- **AnimatedInputLayout**: Input-focused layout with animation support
- **Animator**: 30+ FPS animation engine for smooth rendering

### 🎯 Animation Types

#### Rainbow Animations
```go
// Create rainbow text animation
rainbow := tui.CreateRainbowText("Rainbow Text!", 20)
layout.SetHeaderLine(0, "Rainbow Text!", rainbow)

// Reverse rainbow animation
reverseRainbow := tui.CreateReverseRainbowText("Reverse Rainbow!", 25)
```

#### Pulse Animations
```go
// Create pulsing text with custom color
pulse := tui.CreatePulseText(tui.NewRGB(255, 100, 0), 30)
layout.SetContentLine(0, "Pulsing Status", pulse)
```

#### Custom Animations
```go
// Custom rainbow animation with configuration
customRainbow := &tui.RainbowAnimation{
    Speed:    15,  // Animation speed (lower = faster)
    Length:   10,  // Rainbow length in characters
    Reversed: false,
}
```

## Usage Examples

### Basic Animated Layout
```go
// Create animated layout with 30 FPS
layout := tui.NewAnimatedInputLayout(terminal, 30)

// Set up animated header (2 lines)
layout.SetAnimatedHeader(2)
layout.SetHeaderLine(0, "App Title", tui.CreateRainbowText("App Title", 20))
layout.SetHeaderLine(1, "Subtitle", nil) // Static text

// Set up animated content area (3 lines above input)
layout.SetAnimatedContent(3)
layout.SetContentLine(0, "Status: Ready", tui.CreatePulseText(tui.NewRGB(0, 255, 0), 40))
layout.SetContentLine(1, "Progress: 50%", tui.CreateRainbowText("Progress: 50%", 15))

// Set up animated footer
layout.SetAnimatedFooter(1)
layout.SetFooterLine(0, "Footer Info", tui.CreateRainbowText("Footer Info", 18))

// Start animations
layout.StartAnimations()
defer layout.StopAnimations()
```

### Animated Status Bar
```go
// Create animated status bar
statusBar := tui.NewAnimatedStatusBar(0, 0, 80)
statusBar.AddItem("CPU", "85%", "⚡", tui.CreatePulseText(tui.NewRGB(255, 100, 0), 30), tui.NewStyle())
statusBar.AddItem("Memory", "67%", "🧠", tui.CreateRainbowText("67%", 20), tui.NewStyle())

// Add to animator
layout.GetAnimator().AddElement(statusBar)
```

### Dynamic Content Updates
```go
// Update content dynamically while animations continue
go func() {
    for {
        time.Sleep(500 * time.Millisecond)
        layout.SetContentLine(0, "Status: Processing...", tui.CreatePulseText(tui.NewRGB(255, 255, 0), 40))
        time.Sleep(500 * time.Millisecond)
        layout.SetContentLine(0, "Status: Complete!", tui.CreateRainbowText("Status: Complete!", 15))
    }
}()
```

## Architecture

### Core Components
- **Animator**: Central animation engine managing all animated elements
- **AnimatedElement**: Interface for any component that can be animated
- **TextAnimation**: Interface for text-specific animations
- **AnimatedLayout**: Layout system with built-in animation support

### Animation Types
- **RainbowAnimation**: Moving rainbow effects
- **PulseAnimation**: Brightness pulsing
- **WaveAnimation**: Wave-like color transitions

### Color System
- **RGB**: 24-bit color support
- **SmoothRainbow()**: Generate smooth rainbow gradients
- **MultiGradient()**: Create gradients through multiple color stops
- **RainbowGradient()**: Classic rainbow color progression

## Performance
- **30+ FPS**: Smooth animations at configurable frame rates
- **Efficient Rendering**: Only animated elements are redrawn
- **Concurrent Safe**: Thread-safe animation updates
- **Memory Efficient**: Minimal memory allocation during animation

## Examples
- `tui/examples/simple_animation_demo.go`: Basic feature demonstration
- `tui/examples/demos/animated_demo.go`: Full interactive demo

## Capabilities Delivered
✅ **Multi-line animated content above input area**
✅ **Rainbow text animation with moving colors**
✅ **Animated status bars and footer sections**
✅ **Configurable animation speeds and effects**
✅ **Real-time content updates during animation**
✅ **Full RGB color support with smooth transitions**