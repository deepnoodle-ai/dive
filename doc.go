// Package dive provides a Go library for building AI agents and integrating
// with leading LLMs. It takes a library-first approach, providing a clean API
// for embedding AI capabilities into Go applications.
//
// The core types are:
//
//   - [Agent] orchestrates LLM interactions with tool execution and conversation management.
//   - [Tool] and [TypedTool] define callable tools that an LLM can invoke.
//   - [Response] captures the output from an agent's response generation.
//   - Hook types ([PreGenerationHook], [PostGenerationHook], [PreToolUseHook],
//     [PostToolUseHook]) customize agent behavior at key points.
//
// # Quick Start
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Name:         "Assistant",
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        anthropic.New(),
//	})
//	response, _ := agent.CreateResponse(ctx, dive.WithInput("Hello!"))
//	fmt.Println(response.OutputText())
//
// Built-in tools are available in the [github.com/deepnoodle-ai/dive/toolkit]
// package. LLM providers are in the [github.com/deepnoodle-ai/dive/providers]
// subpackages.
package dive
