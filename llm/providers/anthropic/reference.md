
## Tool Use Deltas

```json
{"type":"message_start","message":{"id":"msg_01KCvnUX6MJRRDB1k3L36tU1","model":"claude-3-7-sonnet-20250219","role":"assistant","content":[],"stop_reason":"","type":"message","usage":{"input_tokens":471,"output_tokens":5}}}
{"type":"content_block_start","index":0,"content_block":{"type":"text"}}
{"type":"ping"}
{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"I'll search for the"}}
{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" top Google hits for Foo Fighters."}}
{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" Let me do that for you right away"}}
{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"."}}
{"type":"content_block_stop","index":0}
{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01VqG2jCdyjoB9HMTFnCKeo2","name":"google_search"}}
{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta"}}
{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\""}}
{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":": \"Foo F"}}
{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"ighter"}}
{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"s\"}"}}
{"type":"content_block_stop","index":1}
{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":0,"output_tokens":80}}
{"type":"message_stop"}
```

## Thinking Block Deltas

```json
{"type":"message_start","message":{"id":"msg_01DfGoUwMtftE8VM22axQ2Jc","type":"message","role":"assistant","model":"claude-3-7-sonnet-20250219","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":778,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":5}}}
{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}
{"type": "ping"}
{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"The user has simply"}}
{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" said \"hey\" as a greeting without any specific"}}
{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" request or question. I don't need to use"}}
{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" any tools for this type of interaction. I should respond with"}}
{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" a friendly greeting and ask how I can help them"}}
{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" today."}}
{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"ErUBCkYIARgCIkACJWxzySUOJzOlZ8WFdgQSbeQA3H1i5rfkKLV9eC4HN6Jcnrlw1NcnPRIPaTs9jLF3rllsfoidjZapJ+5MLwjVEgzTfQ6r9tDZvyBP0nEaDHWRqRWP7CoPOc+ksSIwSlxHzaMbnyXGoiNmCj9thC3N6EDMpemJPx3UZqDtrgmKZgQMMzNzNsh9IxgvtuYKKh1PxXUmWexqlckSVETQj3LbHLK/nwEK+bz0+2b/pA=="}}
{"type":"content_block_stop","index":0}
{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}
{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hello! I'm Assistant, an AI here to help you."}}
{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":" How can I assist you today? Feel free to ask questions"}}
{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":" or let me know what you're looking for, and I'll do my"}}
{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":" best to help."}}
{"type":"content_block_stop","index":1}
{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":104}}
{"type":"message_stop"}
```

## From the Docs

> When passing thinking and redacted_thinking blocks back to the API in a
> multi-turn conversation, you must include the complete unmodified block back
> to the API for the last assistant turn.

> This is critical for maintaining the model’s reasoning flow. We suggest always
> passing back all thinking blocks to the API. For more details, see the
> Preserving thinking blocks section below.
