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
	ChanResponse      chan *llm.Response
	ChanError         chan error
}

func (s *taskState) String() string {
	text, err := executeTemplate(taskStatePromptTemplate, s)
	if err != nil {
		panic(err)
	}
	return text
}
