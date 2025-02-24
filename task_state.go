package dive

import (
	"time"

	"github.com/getstingrai/dive/llm"
)

type taskState struct {
	Task           *Task
	Promise        *Promise
	Status         TaskStatus
	Iterations     int
	Started        time.Time
	Output         string
	Reasoning      string
	ReportedStatus string
	Messages       []*llm.Message
	Suspended      bool
	ChanResponse   chan *llm.Response
	ChanError      chan error
}

func (s *taskState) String() string {
	text, err := executeTemplate(taskStatePromptTemplate, s)
	if err != nil {
		panic(err)
	}
	return text
}
