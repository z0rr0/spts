package args

import (
	"errors"
	"fmt"
	"strconv"
)

// MaxPortNumber is a maximum port number.
const MaxPortNumber uint64 = 65535

// ErrInvalidPort is returned when the port number is invalid.
var ErrInvalidPort = errors.New("invalid port number")

// Port parses a port number.
func Port(value string) (uint16, error) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("parse port: %w", err)
	}

	if port < 1 || port > MaxPortNumber {
		return 0, errors.Join(ErrInvalidPort, fmt.Errorf("port number must be in range [1, %d]", MaxPortNumber))
	}

	return uint16(port), nil
}

// PortHelp returns a port help message.
func PortHelp(port uint16) string {
	return fmt.Sprintf("port to listen on (integer in range 1..%d, default %d)", MaxPortNumber, port)
}

type UserAction uint8

const (
	UserAdd UserAction = iota + 1
	UserUpdate
	UserDelete
	UserVerify
)

const (
	userAdd    = "add"
	userUpdate = "update"
	userDelete = "delete"
	userVerify = "verify"
)

var (
	userActions = map[string]UserAction{
		userAdd:    UserAdd,
		userUpdate: UserUpdate,
		userDelete: UserDelete,
		userVerify: UserVerify,
	}
	allActions = fmt.Sprintf("%s, %s, %s, %s", userAdd, userUpdate, userDelete, userVerify)
)

func UserMod(value string) (UserAction, error) {
	ua, ok := userActions[value]
	if !ok {
		return 0, fmt.Errorf("unknown user action %q, allowed: %s", value, allActions)
	}

	return ua, nil
}

func UserModHelp() string {
	return "user action (" + allActions + ")"
}
