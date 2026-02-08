package main

import "syscall"

type AutoRestartPolicy string

const (
	AutoRestartNever      AutoRestartPolicy = "never"
	AutoRestartAlways     AutoRestartPolicy = "always"
	AutoRestartUnexpected AutoRestartPolicy = "unexpected"
)

type RawTask struct {
	Cmd          string            `yaml:"cmd"`
	NumProcs     *int              `yaml:"numprocs"`
	Umask        *uint32           `yaml:"umask"`
	WorkingDir   string            `yaml:"workingdir"`
	AutoStart    *bool             `yaml:"autostart"`
	AutoRestart  string            `yaml:"autorestart"`
	ExitCodes    any               `yaml:"exitcodes"`
	StartRetries *int              `yaml:"startretries"`
	StartTime    *float64          `yaml:"starttime"`
	StopSignal   string            `yaml:"stopsignal"`
	StopTime     *float64          `yaml:"stoptime"`
	Stdout       string            `yaml:"stdout"`
	Stderr       string            `yaml:"stderr"`
	Env          map[string]string `yaml:"env"`
}

type RawConfig struct {
	Programs map[string]RawTask `yaml:"programs"`
}

type TaskConfig struct {
	name                                           string
	command_line                                   string
	env                                            map[string]string
	working_directory                              string
	num_processes                                  int
	stdout_log_file                                string
	stderr_log_file                                string
	auto_start                                     bool
	seconds_to_wait_before_auto_start              float64
	auto_restart                                   AutoRestartPolicy
	max_auto_restarts                              int
	expected_exit_codes                            []int
	stop_signal                                    syscall.Signal
	seconds_after_stop_request_before_program_kill float64
	umask                                          uint32
}

type Config struct {
	filename string
	tasks    []TaskConfig
}
