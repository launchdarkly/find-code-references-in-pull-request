package github_actions

import (
	"fmt"
	"os"
)

func SetOutput(name, value string) error {
	output := os.Getenv("GITHUB_OUTPUT")

	f, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s=%s\n", name, value)
	return err
}

func LogNotice(format string, a ...any) {
	fmt.Printf("::notice::%s\n", fmt.Sprintf(format, a...))
}

func LogWarning(format string, a ...any) {
	fmt.Printf("::warning::%s\n", fmt.Sprintf(format, a...))
}

func LogError(format string, a ...any) {
	fmt.Printf("::error::%s\n", fmt.Sprintf(format, a...))
}
