package extended

import (
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/toolkit"
)

// Re-export dive helpers for convenience
var (
	NewToolResultError = dive.NewToolResultError
	NewToolResultText  = dive.NewToolResultText
)

// Re-export toolkit types used by extended tools
type PathValidator = toolkit.PathValidator

var NewPathValidator = toolkit.NewPathValidator
