package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type Agent struct {
	sem   sync.Mutex
	c     Config
	cmd   *exec.Cmd
	stdin io.Writer
}

type Config struct {
	UeberzugBinary string
	OnError        func(error)
}

func New(c Config) *Agent {
	if c.UeberzugBinary == "" {
		c.UeberzugBinary = "ueberzug"
	}

	return &Agent{c: c}
}

func (u *Agent) Init() error {
	u.sem.Lock()
	defer u.sem.Unlock()
	return u.init()
}

func (u *Agent) Close() error {
	u.sem.Lock()
	defer u.sem.Unlock()
	if u.cmd == nil {
		return nil
	}

	cmd := u.cmd
	u.cmd = nil

	return cmd.Process.Kill()
}

func (u *Agent) Command(cmd Command) error {
	u.sem.Lock()
	defer u.sem.Unlock()
	if err := u.init(); err != nil {
		return err
	}

	return cmd.JSON(u.stdin)
}

func (u *Agent) init() error {
	if u.cmd != nil {
		return nil
	}

	cmd := exec.Command(
		u.c.UeberzugBinary, "layer",
		"--parser", "json",
		"--loader", "synchronous",
	)
	cmd.Stdout = io.Discard
	stderr, _ := cmd.StderrPipe()
	u.stdin, _ = cmd.StdinPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ueberzug: %w", err)
	}

	go func() {
		dec := json.NewDecoder(stderr)
		for {
			v := Output{}
			if err := dec.Decode(&v); err != nil {
				if err != io.EOF {
					u.c.OnError(err)
					u.Close()
				}
				break
			}
			if err := v.Err(); err != nil {
				u.c.OnError(err)
			}
		}
	}()

	go func() {
		cmd.Wait()
		u.sem.Lock()
		u.cmd = nil
		u.sem.Unlock()
	}()

	u.cmd = cmd
	return nil
}
