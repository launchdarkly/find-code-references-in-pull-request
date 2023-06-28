package github_actions

import (
	"fmt"
	"log"
	"os"
)

func SetOutput(name, value string) error {
	output := os.Getenv("GITHUB_OUTPUT")

	f, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s=%s\n", name, value)
	return err
}
