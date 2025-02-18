package agent

// func TestNewTeam(t *testing.T) {
// 	// Create test agents
// 	agent1 := NewAgent(AgentSpec{
// 		Role:      "researcher",
// 		Goal:      "research topics",
// 		Backstory: "AI researcher",
// 	})
// 	agent2 := NewAgent(AgentSpec{
// 		Role:      "writer",
// 		Goal:      "write content",
// 		Backstory: "Content writer",
// 	})

// 	// Test team creation
// 	team, err := NewTeam(TeamSpec{
// 		Agents: []*Agent{agent1, agent2},
// 	})
// 	require.NoError(t, err)
// 	require.NotNil(t, team)
// 	require.Len(t, team.agents, 2)
// 	require.NotNil(t, team.tasks)
// 	require.NotNil(t, team.state)
// }

// func TestTeamAddTask(t *testing.T) {
// 	team, err := NewTeam(TeamSpec{})
// 	require.NoError(t, err)

// 	t.Run("valid task", func(t *testing.T) {
// 		task := NewTask(TaskSpec{
// 			Name:           "task1",
// 			Description:    "Test task",
// 			ExpectedOutput: "The sum of 1 + 1",
// 			Agent:          NewAgent(AgentSpec{Role: "test"}),
// 		})

// 		err := team.addTask(task)
// 		require.NoError(t, err)
// 		require.Len(t, team.tasks, 1)
// 		require.Equal(t, task, team.tasks["task1"])
// 	})

// 	t.Run("duplicate task", func(t *testing.T) {
// 		task := NewTask(TaskSpec{
// 			Name:        "task1",
// 			Description: "Duplicate task",
// 			Agent:       NewAgent(AgentSpec{Role: "test"}),
// 		})

// 		err := team.addTask(task)
// 		require.Error(t, err)
// 		require.Contains(t, err.Error(), "already exists")
// 	})

// 	t.Run("missing task name", func(t *testing.T) {
// 		task := NewTask(TaskSpec{
// 			Description: "Invalid task",
// 			Agent:       NewAgent(AgentSpec{Role: "test"}),
// 		})

// 		err := team.addTask(task)
// 		require.Error(t, err)
// 		require.Contains(t, err.Error(), "task name is required")
// 	})
// }

// func TestTeamValidate(t *testing.T) {
// 	team, err := NewTeam(TeamSpec{})
// 	require.NoError(t, err)

// 	t.Run("valid dependencies", func(t *testing.T) {
// 		task1 := NewTask(TaskSpec{
// 			Name:        "task1",
// 			Description: "First task",
// 			Agent:       NewAgent(AgentSpec{Role: "test"}),
// 		})
// 		task2 := NewTask(TaskSpec{
// 			Name:         "task2",
// 			Description:  "Second task",
// 			Agent:        NewAgent(AgentSpec{Role: "test"}),
// 			Dependencies: []string{"task1"},
// 		})

// 		require.NoError(t, team.addTask(task1))
// 		require.NoError(t, team.addTask(task2))
// 		require.NoError(t, team.Validate())
// 	})

// 	t.Run("invalid dependency", func(t *testing.T) {
// 		team, err := NewTeam(TeamSpec{})
// 		require.NoError(t, err)

// 		task := NewTask(TaskSpec{
// 			Name:         "task1",
// 			Description:  "Task with invalid dependency",
// 			Agent:        NewAgent(AgentSpec{Role: "test"}),
// 			Dependencies: []string{"non_existent_task"},
// 		})

// 		require.NoError(t, team.addTask(task))
// 		err = team.Validate()
// 		require.Error(t, err)
// 		require.Contains(t, err.Error(), "depends on non-existent task")
// 	})
// }

// func TestTaskValidate(t *testing.T) {
// 	t.Run("valid task", func(t *testing.T) {
// 		task := NewTask(TaskSpec{
// 			Name:        "task1",
// 			Description: "Valid task",
// 			Agent:       NewAgent(AgentSpec{Role: "test"}),
// 			Timeout:     5 * time.Second,
// 		})
// 		require.NoError(t, task.Validate())
// 	})

// 	t.Run("missing required fields", func(t *testing.T) {
// 		testCases := []struct {
// 			name    string
// 			task    *Task
// 			errText string
// 		}{
// 			{
// 				name:    "missing name",
// 				task:    NewTask(TaskSpec{Description: "Test", Agent: NewAgent(AgentSpec{Role: "test"})}),
// 				errText: "task name required",
// 			},
// 			{
// 				name:    "missing description",
// 				task:    NewTask(TaskSpec{Name: "task1", Agent: NewAgent(AgentSpec{Role: "test"})}),
// 				errText: "task description required",
// 			},
// 		}

// 		for _, tc := range testCases {
// 			t.Run(tc.name, func(t *testing.T) {
// 				err := tc.task.Validate()
// 				require.Error(t, err)
// 				require.Contains(t, err.Error(), tc.errText)
// 			})
// 		}
// 	})
// }

// func strMatchesAny(s string, substrings []string) bool {
// 	s = strings.ToLower(s)
// 	for _, substring := range substrings {
// 		if strings.Contains(s, strings.ToLower(substring)) {
// 			return true
// 		}
// 	}
// 	return false
// }

// func TestAdditionTask(t *testing.T) {
// 	worker := NewAgent(AgentSpec{
// 		Role:      "Mathmetician",
// 		Goal:      "Help with math",
// 		Backstory: "Excels at arithmetic",
// 	})

// 	task := NewTask(TaskSpec{
// 		Name:           "t1",
// 		Description:    "The sum of 1 + 1",
// 		ExpectedOutput: "Respond with only the numeric result",
// 		Agent:          worker,
// 	})

// 	team, err := NewTeam(TeamSpec{
// 		Agents: []*Agent{worker},
// 		Tasks:  []*Task{task},
// 	})
// 	require.NoError(t, err)
// 	require.NotNil(t, team)

// 	results, err := team.Execute(context.Background())
// 	require.NoError(t, err)
// 	require.NotNil(t, results)
// 	require.Len(t, results, 1)

// 	t1, ok := results["t1"]
// 	require.True(t, ok)
// 	require.Equal(t, "2", t1.Output.Content)
// }

// func TestTwoTasks(t *testing.T) {
// 	worker := NewAgent(AgentSpec{
// 		Name:      "Mathmetician",
// 		Goal:      "Help with math",
// 		Backstory: "Excels at arithmetic",
// 	})
// 	presenter := NewAgent(AgentSpec{
// 		Name:      "Presenter",
// 		Goal:      "Present the result",
// 		Backstory: "Excels at presenting",
// 	})
// 	addTask := NewTask(TaskSpec{
// 		Name:           "Add",
// 		Description:    "The sum of 1 + 1",
// 		ExpectedOutput: "Respond with only the numeric result",
// 		Agent:          worker,
// 	})
// 	presentTask := NewTask(TaskSpec{
// 		Name:           "Present",
// 		Description:    "Present the result",
// 		ExpectedOutput: "Respond with \"THE RESULT IS <result>\"",
// 		Agent:          presenter,
// 		Dependencies:   []string{"Add"},
// 	})
// 	team, err := NewTeam(TeamSpec{
// 		Agents: []*Agent{worker, presenter},
// 		Tasks:  []*Task{addTask, presentTask},
// 	})
// 	require.NoError(t, err)
// 	require.NotNil(t, team)

// 	results, err := team.Execute(context.Background())
// 	require.NoError(t, err)
// 	require.NotNil(t, results)
// 	require.Len(t, results, 2)

// 	t1, ok := results["Add"]
// 	require.True(t, ok)
// 	require.Equal(t, "2", t1.Output.Content)

// 	t2, ok := results["Present"]
// 	require.True(t, ok)
// 	require.Equal(t, "THE RESULT IS 2", t2.Output.Content)
// }

// func TestDelegation(t *testing.T) {
// 	worker := NewAgent(AgentSpec{
// 		Name:              "Mathmetician",
// 		Goal:              "Help with math",
// 		Backstory:         "Excels at arithmetic",
// 		AcceptsDelegation: true,
// 	})
// 	manager := NewAgent(AgentSpec{
// 		Name:        "Manager",
// 		Goal:        "Manage the team. Do NOT do math yourself.",
// 		Backstory:   "Excels at managing but doesn't do any work directly",
// 		CanDelegate: true,
// 	})
// 	addTask := NewTask(TaskSpec{
// 		Name:           "Add",
// 		Description:    "The sum of 1 + 1",
// 		ExpectedOutput: "Respond with only the numeric result",
// 		Agent:          manager,
// 	})

// 	team, err := NewTeam(TeamSpec{
// 		Agents: []*Agent{worker, manager},
// 		Tasks:  []*Task{addTask},
// 	})
// 	require.NoError(t, err)
// 	require.NotNil(t, team)

// 	results, err := team.Execute(context.Background())
// 	require.NoError(t, err)
// 	require.NotNil(t, results)
// 	require.Len(t, results, 1)

// 	t1, ok := results["Add"]
// 	require.True(t, ok)
// 	require.Equal(t, "2", t1.Output.Content)

// 	require.Len(t, worker.History(), 1)
// 	workerTask := worker.History()[0]
// 	require.True(t, strings.Contains(workerTask.Task.Description(), "1 + 1"))
// 	require.True(t, strings.Contains(workerTask.Output.Content, "2"))
// }
