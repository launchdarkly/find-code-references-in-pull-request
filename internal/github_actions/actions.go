package github_actions

import (
	"fmt"
	"log"
	"os"
)

func SetOutput(name, value string) {
	if err := setOutput(name, value); err != nil {
		SetError("Failed to set outputs.%s\n", name)
	}
}

func setOutput(name, value string) error {
	Debug("setting output %s=%s", name, value)
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
	log.Println(fmt.Sprintf(format, a...))
}

func LogError(err error) {
	log.Println(err)
}

func SetNotice(format string, a ...any) {
	fmt.Printf("::notice::%s\n", fmt.Sprintf(format, a...))
}

func SetWarning(format string, a ...any) {
	fmt.Printf("::warning::%s\n", fmt.Sprintf(format, a...))
}

func SetError(format string, a ...any) {
	fmt.Printf("::error::%s\n", fmt.Sprintf(format, a...))
}

func Debug(format string, a ...any) {
	fmt.Printf("::debug::%s\n", fmt.Sprintf(format, a...))
}

func StartLogGroup(format string, a ...any) {
	fmt.Printf("::group::%s\n", fmt.Sprintf(format, a...))
}

func EndLogGroup() {
	fmt.Println("::endgroup::")
}
