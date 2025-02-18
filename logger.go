package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/getstingrai/agents/llm"
)

type AgentLog struct {
	Name         string `json:"name"`
	Role         string `json:"role"`
	Backstory    string `json:"backstory"`
	Goal         string `json:"goal"`
	SystemPrompt string `json:"system_prompt"`
}

type ConversationLog struct {
	Agent    *AgentLog      `json:"agent"`
	Messages []*llm.Message `json:"messages"`
	Response *llm.Response  `json:"response"`
	// ToolInvocations []*ToolInvocation `json:"tool_invocations"`
}

type FileConversationLogger struct {
	dir       string
	timestamp string
	agentSeq  map[string]int
}

func NewFileConversationLogger(dir string) *FileConversationLogger {
	now := time.Now().Format("2006_01_02_15_04_05")
	dir = path.Join(dir, now)
	return &FileConversationLogger{
		dir:       dir,
		timestamp: now,
		agentSeq:  make(map[string]int),
	}
}

func (l *FileConversationLogger) LogConversation(
	ctx context.Context,
	agent *Agent,
	messages []*llm.Message,
	response *llm.Response,
) error {
	if err := os.MkdirAll(l.dir, 0755); err != nil {
		return err
	}
	l.agentSeq[agent.Name()]++
	seq := l.agentSeq[agent.Name()]

	entry := &ConversationLog{
		Agent: &AgentLog{
			Name:      agent.Name(),
			Role:      agent.Role(),
			Backstory: agent.Backstory(),
			Goal:      agent.Goal(),
		},
		Messages: messages,
		Response: response,
	}
	json, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("%s/%s_conversation_%02d.json", l.dir, agent.Name(), seq)
	if err := os.WriteFile(filename, json, 0644); err != nil {
		return err
	}
	allMessagesText := ""
	for _, message := range messages {
		allMessagesText += "---- " + message.Role.String() + "\n\n" + message.Text() + "\n\n"
	}
	os.WriteFile(fmt.Sprintf("%s/%s_conversation_%02d.txt", l.dir, agent.Name(), seq), []byte(allMessagesText), 0644)
	return nil
}
