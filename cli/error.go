package cli

import "fmt"

type Output struct {
	Message string `json:"message"`
	Name    string `json:"name"`
	Type    string `json:"type"`
}

func (o Output) Err() error {
	if o.Type != "error" {
		return nil
	}
	return fmt.Errorf("[%s] %s", o.Name, o.Message)
}
