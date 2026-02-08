package main

func main() {
	var supervisor Supervisor
	defer supervisor.StopAllTasks()

	supervisor.myConfig = append(supervisor.myConfig, MyTaskConfig{
		name:         "say_hello",
		command:      "echo",
		args:         []string{"hello", "sailor"},
		numProcesses: 3,
	})

	supervisor.myConfig = append(supervisor.myConfig, MyTaskConfig{
		name:         "sleep",
		command:      "sleep",
		args:         []string{"10"},
		numProcesses: 3,
	})

	var shell Shell
	shell.supervisor = &supervisor

	shell.Loop()
}
