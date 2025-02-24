from openai import OpenAI
import json

client = OpenAI()

tools = [
    {
        "type": "function",
        "function": {
            "name": "GoogleSearch",
            "description": "Search Google for the provided query.",
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {"type": "string"},
                },
                "required": ["query"],
                "additionalProperties": False,
            },
            "strict": True,
        },
    }
]

messages = [{"role": "user", "content": "How tall is Mt. St. Helens?"}]

completion = client.chat.completions.create(
    model="gpt-4o",
    messages=messages,
    tools=tools,
    tool_choice="required",
)

choice = completion.choices[0]

tool_call = choice.message.tool_calls[0]

print("message:", choice.message)

print("tool_calls:", choice.message.tool_calls)

messages.append(choice.message)
messages.append(
    {
        "role": "tool",
        "tool_call_id": tool_call.id,
        "content": "9,033 feet",
    }
)

completion = client.chat.completions.create(
    model="gpt-4o",
    messages=messages,
)

print("completion:", completion.choices[0])
