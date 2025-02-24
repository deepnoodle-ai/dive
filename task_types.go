package dive

func NewTaskResultError(task *Task, err error) *TaskResult {
	return &TaskResult{
		Task:  task,
		Error: err,
	}
}
