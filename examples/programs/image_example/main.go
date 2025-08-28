package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/providers/anthropic"
	"github.com/deepnoodle-ai/dive/web"
)

const DefaultImageURL = "https://upload.wikimedia.org/wikipedia/commons/f/f1/Kusuma_bike_large.jpg"

func main() {
	var prompt, url string
	flag.StringVar(&prompt, "prompt", "Describe the image.", "prompt to use")
	flag.StringVar(&url, "url", DefaultImageURL, "url to use")
	flag.Parse()

	ctx := context.Background()

	fetcher := web.NewDefaultBinaryFetcher()
	binary, err := fetcher.FetchBinary(ctx, &web.BinaryFetchInput{URL: url})
	if err != nil {
		log.Fatal(err)
	}

	base64Image := base64.StdEncoding.EncodeToString(binary.Data)

	fmt.Println("Downloaded file", binary.Filename)
	fmt.Println("Content type", binary.ContentType)
	fmt.Println("Size", binary.Size)

	response, err := anthropic.New().Generate(
		ctx,
		llm.WithMessages(
			llm.NewUserTextMessage(prompt),
			llm.NewUserMessage(
				&llm.ImageContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: binary.ContentType,
						Data:      base64Image,
					},
					CacheControl: &llm.CacheControl{
						Type: llm.CacheControlTypeEphemeral,
					},
				},
			),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
