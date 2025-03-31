# Agent Events

## Generation Requests Without Tool Use

A single generation request without tool use results in the following events:

One `llm.request` containing all messages to be sent to the LLM.

If streaming, multiple `llm.event` events, containing incremental LLM generations.

One `llm.response` containing the final LLM response.

## Generation Requests With Tool Use

Generation requests with tool use loop over this sequence of events:

One `llm.request` containing all messages to be sent to the LLM.

If streaming, multiple `llm.event` events, containing incremental LLM generations.

One `llm.response` containing the final LLM response.

Then, for each tool call:

A `tool.called` event containing the tool use ID, tool name, and tool input.

A `tool.output` event containing the tool output.

Then we loop back to start the next generation request, given the tool results.
