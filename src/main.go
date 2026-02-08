package main

import (
	"fmt"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

type ProcessStatus uint
const (
	ProcessStatusStarted  = iota
	ProcessStatusRunning  = iota
	ProcessStatusStopping = iota
	ProcessStatusStopped  = iota
)

func runProcess(ctx context.Context, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("Starting", command)

	cmd.Start()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	status := ProcessStatusStarted
	_ = status

	doneCh := make(chan error, 1)

	go func() {
		doneCh <- cmd.Wait()
	}()

	select {
	case <-signalCh:
		fmt.Println("Shutting down process...")
		_ = cmd.Process.Signal(syscall.SIGTERM)

		select {
		case err := <-doneCh:
			fmt.Println("Process exited gracefully:", err)
			return err;

		case <-time.After(2 * time.Second):
			fmt.Println("Process still exiting, sending SIGKILL...")
			_ = cmd.Process.Kill()
			err := <-doneCh
			fmt.Println("Process killed:", err)

			return err
		}

	case err := <-doneCh:
		fmt.Println("Process exited early:", err)
		return err

	case <-time.After(2 * time.Second):
		status = ProcessStatusRunning
		fmt.Println("Process has sucessfully started")
	}

	return <-doneCh
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// sigCh := make(chan os.Signal, 1)
	// signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// go func() {
	// 	<-sigCh
	// 	fmt.Println("\nShutting down...")
	// 	cancel()
	// }()

	for {
		err := runProcess(ctx, "sleep", "3")

		if err != nil {
			return
		}

		fmt.Println("Restarting in 1 second...")
		time.Sleep(1 * time.Second)
	}
}
