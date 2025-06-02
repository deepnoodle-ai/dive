package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
)

func main() {
	printUserTextMessage()

	// Additional examples for reference
	printConversationExample()
	printToolUseExample()
	printToolResultExample()
	printPDFBase64Example()
	printImageURLExample()
	printImageGenerationReferenceExample()
}

func printUserTextMessage() {
	fmt.Println("\n--- User Text Message Example ---")

	item := responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Role: responses.EasyInputMessageRole("user"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfInputItemContentList: []responses.ResponseInputContentUnionParam{
					{
						OfInputText: &responses.ResponseInputTextParam{
							Text: "Hello, World!",
						},
					},
				},
			},
		},
	}
	printJSON(item)
}

func printJSON(item any) {
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
}

// printConversationExample demonstrates a back-and-forth between a user and the assistant.
func printConversationExample() {
	fmt.Println("\n--- Conversation Example ---")

	conversation := []responses.ResponseInputItemUnionParam{
		// User message
		{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRole("user"),
				Content: responses.EasyInputMessageContentUnionParam{
					OfInputItemContentList: []responses.ResponseInputContentUnionParam{
						{
							OfInputText: &responses.ResponseInputTextParam{Text: "What is the capital of France?"},
						},
					},
				},
			},
		},
		// Assistant response
		{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRole("assistant"),
				Content: responses.EasyInputMessageContentUnionParam{
					OfInputItemContentList: []responses.ResponseInputContentUnionParam{
						{
							OfInputText: &responses.ResponseInputTextParam{Text: "The capital of France is Paris."},
						},
					},
				},
			},
		},
	}
	printJSON(conversation)
}

// printToolUseExample demonstrates sending a tool call from the assistant.
func printToolUseExample() {
	fmt.Println("\n--- Tool Use Example ---")

	item := responses.ResponseInputItemParamOfFunctionCall(
		"{\"location\":\"San Francisco\"}", // arguments
		"call-1",                           // call ID
		"weather",                          // function / tool name
	)
	printJSON(item)
}

// printToolResultExample demonstrates returning a tool result back to the model.
func printToolResultExample() {
	fmt.Println("\n--- Tool Result Example ---")

	item := responses.ResponseInputItemParamOfFunctionCallOutput(
		"call-1",                 // call ID (must match the tool use)
		"{\"temperature\":65}\n", // tool output as JSON string (example)
	)
	printJSON(item)
}

// printPDFBase64Example demonstrates attaching a PDF file as base64 data in a message.
func printPDFBase64Example() {
	fmt.Println("\n--- PDF Base64 Example ---")

	// Example (truncated) base64-encoded PDF data – in real usage, provide full data.
	pdfData := "JVBERi0xLjQKJcTl8uXr="
	dataURL := "data:application/pdf;base64," + pdfData

	fileParam := responses.ResponseInputFileParam{
		Filename: openai.String("example.pdf"),
		FileData: openai.String(dataURL),
	}

	item := responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Role: responses.EasyInputMessageRole("user"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfInputItemContentList: []responses.ResponseInputContentUnionParam{
					{
						OfInputText: &responses.ResponseInputTextParam{Text: "Please summarize the attached PDF."},
					},
					{
						OfInputFile: &fileParam,
					},
				},
			},
		},
	}
	printJSON(item)
}

// printImageURLExample demonstrates referencing an external image via URL.
func printImageURLExample() {
	fmt.Println("\n--- Image URL Example ---")

	imageParam := responses.ResponseInputImageParam{
		ImageURL: openai.String("https://example.com/cat.png"),
		Detail:   responses.ResponseInputImageDetailAuto,
	}

	item := responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Role: responses.EasyInputMessageRole("user"),
			Content: responses.EasyInputMessageContentUnionParam{
				OfInputItemContentList: []responses.ResponseInputContentUnionParam{
					{
						OfInputText: &responses.ResponseInputTextParam{Text: "Describe this image."},
					},
					{
						OfInputImage: &imageParam,
					},
				},
			},
		},
	}
	printJSON(item)
}

// printImageGenerationReferenceExample demonstrates sending back a reference to an image that was generated earlier.
func printImageGenerationReferenceExample() {
	fmt.Println("\n--- Image Generation Reference Example ---")

	item := responses.ResponseInputItemParamOfImageGenerationCall(
		"img-abc123", // generation ID returned by the prior generation call
		"",           // result intentionally left blank for reference
		"succeeded",  // generation status
	)
	printJSON(item)
}
