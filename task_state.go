package dive

import (
	"time"

	"github.com/getstingrai/dive/llm"
)

type taskState struct {
	Task              *Task
	Publisher         *StreamPublisher
	Status            TaskStatus
	Iterations        int
	Started           time.Time
	Output            string
	Reasoning         string
	StatusDescription string
	Messages          []*llm.Message
	Paused            bool
	Usage             llm.Usage
}

func (s *taskState) String() string {
	text, err := executeTemplate(taskStatePromptTemplate, s)
	if err != nil {
		panic(err)
	}
	return text
}
