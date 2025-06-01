package llm

import (
	"encoding/base64"
	"fmt"
	"net/http"
)

type ImageType string

const (
	ImageTypePNG  ImageType = "image/png"
	ImageTypeJPEG ImageType = "image/jpeg"
	ImageTypeGIF  ImageType = "image/gif"
	ImageTypeWEBP ImageType = "image/webp"
)

// DetectImageType detects the type of an image from its base64-encoded data.
// Supports PNG, JPEG, GIF, and WEBP.
func DetectImageType(imageBase64 string) (ImageType, error) {
	imageData, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return "", err
	}
	imageType := http.DetectContentType(imageData)
	switch imageType {
	case "image/png":
		return ImageTypePNG, nil
	case "image/jpeg":
		return ImageTypeJPEG, nil
	case "image/gif":
		return ImageTypeGIF, nil
	case "image/webp":
		return ImageTypeWEBP, nil
	}
	return "", fmt.Errorf("unknown image type - content type: %s", imageType)
}
