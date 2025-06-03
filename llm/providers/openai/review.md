
Okay, I've conducted a review of `llm/providers/openai/decode.go` and `llm/providers/openai/encode.go`, comparing them against `llm/content.go` and the general principles you've outlined. Here are my notes, broken down by file and then general observations.

## Review of `llm/providers/openai/decode.go`

This file is responsible for converting OpenAI's API responses into Dive's `llm.Response` and `llm.Content` types.

**`decodeAssistantResponse` Function:**
*   **Usage:** Correctly maps `InputTokens`, `OutputTokens`, and `CacheReadInputTokens` from `response.Usage`.
*   **StopReason:** The `determineStopReason` function (located in `provider.go`) has basic logic:
    *   Identifies "tool\_use" if any output item type ends with `_call`.
    *   Defaults to "end\_turn" if `response.Status == "completed"` without tool calls, or for other scenarios.
    *   **Note/Follow-up:** This could be enhanced. OpenAI's `response.IncompleteDetails.Reason` can provide more specific reasons like "max\_output\_tokens" or "content\_filter". Consider mapping these to more specific `llm.StopReason` values in Dive.

**`decodeResponseItem` Function (and its sub-decoders):**

1.  **`case "message"` -> `decodeMessageContent`:**
    *   **`output_text`**: Maps to `llm.TextContent`.
        *   **Citations/Annotations**: `url_citation` is mapped to `llm.WebSearchResultLocation`. This is good.
        *   **Note/Follow-up**: The `StartIndex` and `EndIndex` for citations are commented out in the mapping. If precise text highlighting for citations is required by Dive, these should be mapped from `urlCitation.StartIndex` and `urlCitation.EndIndex` to the corresponding fields in `llm.WebSearchResultLocation` (or a more general `llm.Citation` type).
    *   **`refusal`**: Maps to `llm.RefusalContent`. Correct.

2.  **`case "function_call"` -> `decodeFunctionCallContent`:**
    *   Maps to `llm.ToolUseContent`.
    *   Currently, `llm.ToolUseContent.ID` is set from `functionCall.CallID`.
    *   OpenAI's `responses.ResponseFunctionToolCall` has:
        *   `ID`: "The unique ID of the function tool call."
        *   `CallID`: "An identifier used when responding to the tool call with output."
    *   **Note/Follow-up (Critical):** For semantic correctness, `llm.ToolUseContent.ID` (representing the unique ID of the tool invocation itself) should likely be mapped from `functionCall.ID`. The `functionCall.CallID` is what the client (Dive) would use in the `tool_use_id` field when sending back the `ToolResultContent`. This needs careful verification against how `ToolResultContent.ToolUseID` is populated and used in the encoding step. The current mapping might lead to issues in linking tool calls to their results if `ID` and `CallID` are distinct and serve different purposes.

3.  **`case "image_generation_call"` -> `decodeImageGenerationCallContent`:**
    *   Maps to `llm.ImageContent` with `Source` type `llm.ContentSourceTypeBase64`.
    *   Correctly uses `imgCall.ID` for `GenerationID` and `imgCall.Status` for `GenerationStatus`.
    *   Includes image type detection. Good.

4.  **`case "web_search_call"` -> `decodeWebSearchCallContent`:**
    *   Currently maps to `llm.WebSearchToolResultContent{ToolUseID: call.ID, Content: nil}`.
    *   **Note/Follow-up (Major):** This is semantically incorrect. A `web_search_call` in the assistant's output indicates the assistant *intends to perform a web search*. This is a *tool use*, not a *tool result*.
    *   It should map to an `llm.ToolUseContent` (or `llm.ServerToolUseContent`) with `Name: "web_search"`.
    *   The challenge is that `responses.ResponseFunctionWebSearch` (the OpenAI type for this output item) does not directly contain the search query. The query is usually inferred from prior text outputs by the assistant.
    *   **Recommendation**: Decode to `llm.ToolUseContent{ID: call.ID, Name: "web_search", Input: []byte("{}")}`. The calling system in Dive would then be responsible for finding the query from context if it needs to execute the search.

5.  **`case "mcp_call"` -> `decodeMcpCallContent`:**
    *   Currently maps to `llm.TextContent` displaying `mcpCall.Output`.
    *   **Note/Follow-up (Major):** This is incorrect for representing a tool call. An `mcp_call` from the assistant is a tool invocation.
    *   Should map to `llm.MCPToolUseContent`:
        *   `ID`: `mcpCall.ID`
        *   `Name`: `mcpCall.Name`
        *   `ServerName`: `mcpCall.ServerLabel`
        *   `Input`: `[]byte(mcpCall.Arguments)`
    *   The `responses.ResponseOutputItemMcpCall` also has `Output` and `Error` fields. If `Output` is present, it might mean OpenAI's MCP can execute and return the result in the same block. Dive's model separates `ToolUse` and `ToolResult`. If this is the case, this single OpenAI item might need to be decoded into both an `MCPToolUseContent` and an `MCPToolResultContent` for Dive, or Dive's content model for MCP needs adjustment. For now, assume it's an invocation request first.

6.  **`case "mcp_list_tools"` -> `decodeMcpListToolsContent`:**
    *   Maps to `llm.TextContent` summarizing the tools.
    *   **Note:** Acceptable for display. If Dive needs to act programmatically on this list, a structured `llm.MCPToolsListContent` type could be added to `llm/content.go`.

7.  **`case "mcp_approval_request"` -> `decodeMcpApprovalRequestContent`:**
    *   Maps to `llm.TextContent` describing the approval request.
    *   **Note:** Acceptable for display. Similar to `mcp_list_tools`, a structured `llm.MCPApprovalRequestContent` could be added if Dive needs to automate approval flows.

8.  **`case "reasoning"` -> `decodeReasoningContent`:**
    *   Maps to `llm.ThinkingContent`, using `reasoning.Summary[].Text` for `Thinking` and `reasoning.EncryptedContent` for `Signature`. Correct.

9.  **`case "file_search_call"` -> `decodeFileSearchCallContent`:**
    *   Maps to `llm.ToolUseContent` with `Name: "file_search"`.
    *   Currently sets `Input: []byte("{}")`.
    *   **Note/Follow-up:** The `Input` should be populated from `fileSearchCall.Queries`. E.g., `json.Marshal(map[string]any{"queries": fileSearchCall.Queries})`.

10. **`case "computer_call"` -> `decodeComputerCallContent`:**
    *   Maps to `llm.ToolUseContent` with `Name: "computer"`.
    *   `Input` is correctly set by marshalling `computerCall.Action`. Good.

11. **`case "code_interpreter_call"` -> `decodeCodeInterpreterCallContent`:**
    *   Maps to `llm.ToolUseContent` with `Name: "code_interpreter"`.
    *   Currently, `Input` is derived from `codeCall.Results` if present, otherwise `{}`.
    *   **Note/Follow-up (Critical):** The *input* to a code interpreter tool use is the *code to be executed*, which is in `codeCall.Code`. The `codeCall.Results` are outputs.
    *   `Input` should be `json.Marshal(map[string]any{"code": codeCall.Code})`. If `codeCall.Results` are present in this output item from the assistant, it's an unusual pattern (assistant providing results for a call it's making). Standard flow is assistant requests code execution, client runs it, client sends results back.

12. **`case "local_shell_call"` -> `decodeLocalShellCallContent`:**
    *   Maps to `llm.ToolUseContent` with `Name: "local_shell"`.
    *   Currently sets `Input: []byte("{}")`.
    *   **Note/Follow-up:** `Input` should be populated by marshalling `shellCall.Action` (which contains `Command`, `Env`, etc.), similar to `computer_call`.

## Review of `llm/providers/openai/encode.go`

This file converts Dive's `llm.Message` and `llm.Content` types into OpenAI's API request parameters.

**`encodeAssistantContent` Function:**

1.  **`*llm.TextContent`**:
    *   Maps to `responses.ResponseOutputTextParam`.
    *   **Note/Follow-up**: `llm.TextContent.Citations` are not encoded back into OpenAI annotations. If an assistant message being sent as history contains citations, these should be translated.

2.  **`*llm.RefusalContent`**: Maps to `responses.ResponseOutputRefusalParam`. Correct.

3.  **`*llm.ImageContent`**:
    *   Encodes as a reference using `responses.ResponseInputItemParamOfImageGenerationCall` with `GenerationID`. This implies the image was previously generated and is being referenced in the input history.
    *   **Note**: This is specific. If an assistant is constructing a message with new image data (not from a tool call), OpenAI's input format for this scenario needs to be checked. Usually, assistants *output* images, users *input* them.

4.  **`*llm.DocumentContent` / `*llm.FileContent`**:
    *   Converted to a simple text message summary.
    *   **Note**: This is a lossy conversion. If OpenAI supports richer ways for an assistant to reference documents/files in its *input context* (e.g., when sending message history), those should be used.

5.  **`*llm.ToolUseContent`**:
    *   Maps to `responses.ResponseInputItemParamOfFunctionCall`. `c.ID` becomes `CallID`, `c.Name` becomes `Name`, `c.Input` becomes `Arguments`.
    *   **Note**: This seems consistent with `CallID` being the linking ID if `llm.ToolUseContent.ID` stores this.

6.  **`*llm.ToolResultContent` (in assistant message context)**:
    *   This is unusual. If an assistant message includes a `ToolResultContent` (e.g., summarizing a past tool interaction it performed), it's encoded as `responses.ResponseInputItemParamOfFunctionCallOutput`.
    *   `IsError` from `llm.ToolResultContent` is ignored, which is consistent with OpenAI's expectation of errors being in the output string.

7.  **`*llm.ServerToolUseContent`**:
    *   Mapped to a generic `responses.ResponseInputItemParamOfFunctionCall`.
    *   **Note**: This is a fallback. Built-in tools like `web_search` or `file_search` might have more specific representations if OpenAI supports them in the input from an assistant's prior turn.

8.  **`*llm.WebSearchToolResultContent` (in assistant message context)**:
    *   Encoded as `responses.ResponseInputItemParamOfWebSearchCall` marking it completed.
    *   **Note/Follow-up (Major):** The actual search result content (`c.Content []*WebSearchResult`) is lost. This encoding is just a status marker. If an assistant is meant to be "stating" web results as part of its turn (for context), these should likely be formatted into an `llm.TextContent` with citations. This path in `encodeAssistantContent` is for when `WebSearchToolResultContent` is part of an *assistant's own message* being sent as history.

9.  **`*llm.ThinkingContent` / `*llm.RedactedThinkingContent`**: Mapped correctly to `responses.ResponseReasoningItemParam`.

10. **`*llm.MCPToolUseContent` (in assistant message context)**:
    *   Encoded as a generic `responses.ResponseInputItemParamOfFunctionCall`.
    *   **Note/Follow-up**: `ServerName` is lost. The name could be prefixed (e.g., `server_name/tool_name`) if this distinction is important and OpenAI has no better mechanism for MCP tool use history from assistants.

11. **`*llm.MCPToolResultContent` (in assistant message context)**:
    *   Encoded as `responses.ResponseInputItemParamOfFunctionCallOutput`.
    *   **Note/Follow-up**: Structured `ContentChunk` data is flattened into a single string. `IsError` is handled by prepending "Error: ".

12. **`*llm.CodeExecutionToolResultContent` (in assistant message context)**:
    *   Marshals `c.Content` (the `CodeExecutionResult` struct) to JSON and encodes as `responses.ResponseInputItemParamOfFunctionCallOutput`.
    *   **Note**: This is a reasonable fallback.

**`encodeUserContent` Function:**

1.  **`*llm.TextContent`**: Maps to `responses.ResponseInputTextParam`. Correct. `Citations` are appropriately ignored for user input.
2.  **`*llm.ImageContent`**: Maps correctly to `responses.ResponseInputImageParam` using data URLs, file IDs, or external URLs.
3.  **`*llm.DocumentContent`**:
    *   Maps to `responses.ResponseInputFileParam`.
    *   URL-based document sources (`ContentSourceTypeURL`) are explicitly not supported, throwing an error.
    *   **Note**: This limitation for URL documents should be documented or revisited if OpenAI's file input mechanisms evolve.

**`encodeToolResultContent` Function (for user "tool_result" message type):**
*   Maps `llm.ToolResultContent` to `responses.ResponseInputItemParamOfFunctionCallOutput`.
*   `Content` is marshalled to JSON if not already string/bytes.
*   `IsError` is ignored (OpenAI convention). Correct.

## General Observations & `llm/content.go` Considerations

1.  **Tool Call ID Consistency (`llm.ToolUseContent.ID`):**
    *   As highlighted in `decodeFunctionCallContent`, the exact meaning and mapping of `llm.ToolUseContent.ID` versus OpenAI's `ID` and `CallID` for function calls needs to be crystal clear and consistently applied in both encoding and decoding. The current setup seems to use `llm.ToolUseContent.ID` as the equivalent of OpenAI's `CallID` (for linking results). If this is the intent, it should be documented. Using OpenAI's `functionCall.ID` (unique ID of the call itself) for `llm.ToolUseContent.ID` might be more semantically pure for representing the invocation, and then `ToolResultContent.ToolUseID` would refer to this.

2.  **Assistant "Echoing" Results:**
    *   Several `encodeAssistantContent` cases handle situations where an assistant's message contains content types usually provided by the user/client (e.g., `ToolResultContent`, `WebSearchToolResultContent`).
    *   The current encoding for these often involves simplification or data loss (e.g., structured web results become a simple "completed" status).
    *   **Recommendation**: If an assistant needs to convey past results as context, the preferred Dive approach should be to format this information into an `llm.TextContent` (possibly with `Citations`), rather than trying to re-encode a "result" content block as if the assistant is *providing* a tool result in the input stream. The current handlers might be fallbacks for rare cases or direct mappings from other LLM providers that allow this.

3.  **`llm/content.go` Extensibility for OpenAI specifics:**
    *   **`web_search_call` (Assistant Output):** As Dive favors Anthropic, where a `server_tool_use` for web search includes the query, OpenAI's `web_search_call` (which lacks the query) is awkward. No new `llm.Content` type seems immediately necessary if `ToolUseContent` with an empty/placeholder input is used for the invocation.
    *   **MCP Items:** `llm.MCPToolUseContent` and `llm.MCPToolResultContent` exist. For `mcp_list_tools` and `mcp_approval_request` from OpenAI, current mapping to `TextContent` is okay. If more programmatic interaction is needed in Dive, new types like `llm.MCPToolsListContent` and `llm.MCPApprovalRequestContent` could be added to `llm/content.go`.
    *   **File Search Results:** OpenAI's `FileSearchToolCall` returns structured results. `llm.ToolResultContent` is generic. A dedicated `llm.FileSearchToolResultContent` (similar to `WebSearchToolResultContent`) could be beneficial for type safety and explicit handling within Dive, but marshalling into the generic `ToolResultContent.Content` is also a viable, flexible approach.
    *   **Code Interpreter Results:** OpenAI's `CodeInterpreterToolCall` returns `Results` which can be logs or files. `llm.CodeExecutionToolResultContent` (from `content.go`) is more focused on stdout/stderr/return_code. For OpenAI, marshalling its `Results` structure into `ToolResultContent.Content` (as done for `ToolResultContent` generally) is a good generic solution.

4.  **Lossy Conversions:**
    *   Encoding `DocumentContent` and `FileContent` from assistant messages into plain text is lossy. This is a known trade-off when message history from one LLM (that might support rich object passthrough) is fed to another that primarily expects text or specific tool call structures.
    *   Encoding `WebSearchToolResultContent` (from assistant) loses the actual search snippets.
    *   Encoding `MCPToolResultContent` (from assistant) flattens chunks.

## Summary of Key Action Items/Follow-ups:

*   **Critical:** Re-evaluate and ensure consistency in handling `ID` vs. `CallID` for `llm.ToolUseContent` when decoding OpenAI's `function_call` and when encoding `llm.ToolUseContent` back.
*   **Critical:** Correct the input mapping for tool invocations in `decode.go`:
    *   `file_search_call`: Use `fileSearchCall.Queries` for `Input`.
    *   `code_interpreter_call`: Use `codeCall.Code` for `Input`.
    *   `local_shell_call`: Use `shellCall.Action` for `Input`.
*   **Major:** Change decoding of assistant's `web_search_call` from `WebSearchToolResultContent` to `ToolUseContent` (with placeholder input).
*   **Major:** Change decoding of assistant's `mcp_call` from `TextContent` to `MCPToolUseContent`.
*   **Review:** Consider if/how to encode `llm.TextContent.Citations` back to OpenAI annotations when sending assistant messages as history.
*   **Review:** The strategy for assistant messages containing "result-like" content (e.g., `WebSearchToolResultContent`). Formatting into `TextContent` is often better than lossy direct encoding.
*   **Minor Enhancement:** Map OpenAI's `response.IncompleteDetails.Reason` to more specific `llm.StopReason` values.
*   **Documentation/Consideration:** Note the lossy conversions (Document/File content in assistant messages, MCP result chunks) and limitations (URL-based documents for user input).
*   **Consideration:** Whether to add more specific structured `llm.Content` types for `mcp_list_tools`, `mcp_approval_request`, or `FileSearchToolResult` if current mappings prove insufficient for Dive's internal logic.

This review should provide a solid basis for the follow-up work to align the OpenAI provider more closely with Dive's abstractions and ensure robust handling of various content types.
