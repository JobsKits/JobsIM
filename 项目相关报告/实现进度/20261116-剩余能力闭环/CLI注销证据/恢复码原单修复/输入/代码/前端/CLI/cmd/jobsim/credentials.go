// Created by Jobs on 2026年11月14日，星期六.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func secretInput(reader *bufio.Reader, label string) ([]byte, error) {
	fmt.Print(label)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		return value, err
	}
	value, err := reader.ReadString('\n')
	return []byte(strings.TrimSuffix(strings.TrimSuffix(value, "\n"), "\r")), err
}
func erase(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
func (c *client) recoverAccount() error {
	fmt.Print("Username: ")
	username, err := c.reader.ReadString('\n')
	if err != nil {
		return err
	}
	code, err := secretInput(c.reader, "Recovery code: ")
	if err != nil {
		return err
	}
	defer erase(code)
	password, err := secretInput(c.reader, "New password: ")
	if err != nil {
		return err
	}
	defer erase(password)
	var result any
	err = c.call("POST", "/api/auth/recover", map[string]string{"username": strings.TrimSpace(username), "code": string(code), "password": string(password), "requestId": id()}, &result)
	if err != nil {
		return err
	}
	return printJSON(result)
}
func (c *client) recoveryCodes() error {
	password, err := secretInput(c.reader, "Current password (codes shown once): ")
	if err != nil {
		return err
	}
	defer erase(password)
	var result any
	if err = c.call("POST", "/api/auth/recovery-codes", map[string]string{"password": string(password), "requestId": id()}, &result); err != nil {
		return err
	}
	return printJSON(result)
}
