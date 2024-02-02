package github_actions

import (
	"fmt"
	"os"
)

func SetOutputOrLogError(name, value string) {
	if err := SetOutput(name, value); err != nil {
		LogError("Failed to set outputs.%s\n", name)
	}
}

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

func MaskInput(input string) {
	fmt.Printf("::add-mask::%s\n", input)
}

func Log(format string, a ...any) {
	fmt.Printf(format, a...)
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

func LogDebug(format string, a ...any) {
	fmt.Printf("::debug::%s\n", fmt.Sprintf(format, a...))
}

func StartLogGroup(format string, a ...any) {
	fmt.Printf("::group::%s\n", fmt.Sprintf(format, a...))
}

func EndLogGroup() {
	fmt.Println("::endgroup::")
}
