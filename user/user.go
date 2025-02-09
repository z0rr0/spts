package user

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"golang.org/x/term"

	"github.com/z0rr0/spts/args"
	"github.com/z0rr0/spts/auth"
)

var (
	ErrPrompt = errors.New("prompt error")
)

type TerminalReader interface {
	ReadLine() (string, error)
	ReadPassword(prompt string) (string, error)
	SetPrompt(prompt string)
	Write(buf []byte) (n int, err error)
}

type Controller struct {
	terminal TerminalReader
	auth     auth.Authenticator
}

func (c *Controller) enterAndRepeatPassword() (string, error) {
	var (
		password string
		repeat   string
		err      error
	)

	for {
		password, err = c.terminal.ReadPassword("Enter password: ")
		if err != nil {
			return "", errors.Join(ErrPrompt, fmt.Errorf("read password: %w", err))
		}

		repeat, err = c.terminal.ReadPassword("Repeat password: ")
		if err != nil {
			return "", errors.Join(ErrPrompt, fmt.Errorf("read password: %w", err))
		}

		isEqual := subtle.ConstantTimeCompare([]byte(password), []byte(repeat))
		if isEqual == 1 {
			return password, nil
		}
		fmt.Println("Passwords do not match, try again")
	}
}

func (c *Controller) Add() error {
	c.terminal.SetPrompt("Enter username: ")
	username, err := c.terminal.ReadLine()
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("read username: %w", err))
	}

	exists, err := c.auth.UserExists(username)
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("check user: %w", err))
	}

	if exists {
		return fmt.Errorf("user %q already exists", username)
	}

	password, err := c.enterAndRepeatPassword()
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("enter password: %w", err))
	}

	if err = c.auth.Register(username, password); err != nil {
		return fmt.Errorf("register user: %w", err)
	}

	return nil
}

func (c *Controller) Update() error {
	c.terminal.SetPrompt("Enter username: ")
	username, err := c.terminal.ReadLine()
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("read username: %w", err))
	}

	exists, err := c.auth.UserExists(username)
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("check user: %w", err))
	}

	if !exists {
		return fmt.Errorf("user %q not found", username)
	}

	password, err := c.enterAndRepeatPassword()
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("enter password: %w", err))
	}

	if err = c.auth.Update(username, password); err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	return nil
}

func (c *Controller) Delete() error {
	c.terminal.SetPrompt("Enter username: ")
	username, err := c.terminal.ReadLine()
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("read username: %w", err))
	}

	if err = c.auth.Delete(username); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}

func (c *Controller) Verify() error {
	c.terminal.SetPrompt("Enter username: ")
	username, err := c.terminal.ReadLine()
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("read username: %w", err))
	}

	exists, err := c.auth.UserExists(username)
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("check user: %w", err))
	}

	if !exists {
		return fmt.Errorf("user %q not found", username)
	}

	password, err := c.terminal.ReadPassword("Enter password: ")
	if err != nil {
		return errors.Join(ErrPrompt, fmt.Errorf("read password: %w", err))
	}

	if c.auth.Verify(username, password) {
		if _, err = c.terminal.Write([]byte("Password is correct\n")); err != nil {
			return errors.Join(ErrPrompt, fmt.Errorf("write message: %w", err))
		}
	} else {
		if _, err = c.terminal.Write([]byte("Password is incorrect\n")); err != nil {
			return errors.Join(ErrPrompt, fmt.Errorf("write message: %w", err))
		}
	}

	return nil
}

func NewTerminal(reader io.ReadWriter) (TerminalReader, func(), error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, nil, errors.Join(ErrPrompt, fmt.Errorf("set raw mode: %w", err))
	}

	cleanup := func() {
		if err = term.Restore(int(os.Stdin.Fd()), oldState); err != nil {
			slog.Error("failed to restore terminal state", "error", err)
		}
	}

	return term.NewTerminal(reader, ""), cleanup, nil
}

func NewController(reader io.ReadWriter, a auth.Authenticator) (*Controller, func(), error) {
	t, cleanup, err := NewTerminal(reader)
	if err != nil {
		return nil, nil, errors.Join(ErrPrompt, fmt.Errorf("new terminal: %w", err))
	}

	return &Controller{terminal: t, auth: a}, cleanup, nil
}

func Action(u args.UserAction, a auth.Authenticator) error {
	c, cleanup, err := NewController(os.Stdin, a)
	if err != nil {
		return err
	}

	defer cleanup()

	switch u {
	case args.UserAdd:
		return c.Add()
	case args.UserUpdate:
		return c.Update()
	case args.UserDelete:
		return c.Delete()
	case args.UserVerify:
		return c.Verify()
	}

	return fmt.Errorf("unknown user action %q", u)
}
