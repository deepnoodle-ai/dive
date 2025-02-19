
# Agent Execution

- The agent may have 0-N long-running tasks (work) assigned to it. The agent should "remember" all these tasks and try to move each task forward. There should be a limit set which is the number of tasks that may be active at a time, which is usually set to 1.

- The agent should track the status of each task: queued, in-progress, complete, or error.

- When a task is completed (or errored), the tasks promise should be fired with the result.

- The agent run loop should have a periodic tick frequency defaulting to 1 second. On each tick, the agent should attempt to move forward any assigned tasks. E.g. if no task is active, a task should be pulled from the queue, activated, and work started on it.

- LLM interaction(s) should be used to decide whether each task has been completed, after some work on a task finishes.

- The agent will need to have a workspace with 0-N documents as a working memory. Task state should be reflected there.

- Tasks may have a priority, so higher prio ones are acted on before lower prio ones.
