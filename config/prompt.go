package config

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// PromptPassword reads a password from the controlling terminal without
// echoing it, for use as ResolvePassword's promptFn.
func PromptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(pw), nil
}
