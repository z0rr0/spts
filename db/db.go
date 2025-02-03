package db

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
)

var (
	// ErrReadUsers is an error for failed to load users.
	ErrReadUsers = fmt.Errorf("read users error")

	// ErrWriteUsers is an error for failed to save users.
	ErrWriteUsers = fmt.Errorf("write users error")

	// ErrUserNotFound is an error for user not found.
	ErrUserNotFound = fmt.Errorf("user not found")

	// ErrInconsistentRequest is an error for inconsistent requests.
	ErrInconsistentRequest = fmt.Errorf("inconsistent request")
)

// UserStorage is a user storage interface.
type UserStorage interface {
	Add(username, salt, hash string) error
	Delete(username string) error
	Update(username, salt, hash string) error
	Get(username string) (User, error)
	List() []User
}

// User is a user record.
type User struct {
	Username string // unique username
	Salt     string // random salt
	Hash     string // password hash
}

// record returns user record as a slice of strings.
func (u *User) record() []string {
	return []string{u.Username, u.Salt, u.Hash}
}

// NewUser creates a new user.
func NewUser(username, salt, hash string) (*User, error) {
	if username == "" {
		return nil, errors.Join(ErrInconsistentRequest, errors.New("empty username"))
	}

	if salt == "" {
		return nil, errors.Join(ErrInconsistentRequest, errors.New("empty salt"))
	}

	if hash == "" {
		return nil, errors.Join(ErrInconsistentRequest, errors.New("empty hash"))
	}

	return &User{Username: username, Salt: salt, Hash: hash}, nil
}

// UserStore is in-memory user storage.
type UserStore struct {
	mu       sync.RWMutex
	users    map[string]User
	filename string
}

// NewStorage creates a new user storage.
func NewStorage(filename string) (*UserStore, error) {
	store := &UserStore{users: make(map[string]User), filename: filename}

	if err := store.load(); err != nil {
		return nil, fmt.Errorf("failed to load users: %w", err)
	}

	return store, nil
}

// load reads users from CSV file.
func (s *UserStore) load() error {
	const fieldsLen = 3

	file, err := os.OpenFile(s.filename, os.O_RDONLY|os.O_CREATE, 0600)
	if err != nil {
		return errors.Join(ErrReadUsers, fmt.Errorf("failed to open file: %w", err))
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.Error("failed to close file", "name", s.filename, "error", closeErr)
		}
	}()

	// file format example:
	// # username,salt,hash
	// user1,salt1,hash1
	reader := csv.NewReader(file)
	reader.Comment = '#'

	records, err := reader.ReadAll()
	if err != nil {
		return errors.Join(ErrReadUsers, fmt.Errorf("failed to read records: %w", err))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, record := range records {
		// skip invalid records
		if len(record) == fieldsLen {
			if user, errUser := NewUser(record[0], record[1], record[2]); errUser != nil {
				slog.Error("failed to create user", "record", record, "error", errUser)
			} else {
				s.users[user.Username] = *user
			}
		}
	}

	return nil
}

// save writes users to CSV file.
// Caller must hold the lock before calling this function.
func (s *UserStore) save() error {
	file, err := os.CreateTemp("", "users.csv")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	users := s.sortedUsers()
	writer := csv.NewWriter(file)

	if err = writer.Write([]string{"#username", "salt", "hash"}); err != nil {
		return errors.Join(ErrWriteUsers, fmt.Errorf("failed to write header: %w", err))
	}

	for _, user := range users {
		if err = writer.Write(user.record()); err != nil {
			return errors.Join(ErrWriteUsers, fmt.Errorf("failed to write record: %w", err))
		}
	}

	writer.Flush()
	if err = writer.Error(); err != nil {
		return errors.Join(ErrWriteUsers, fmt.Errorf("failed to flush writer: %w", err))
	}

	tmpFileName := file.Name()
	if err = file.Close(); err != nil {
		return errors.Join(ErrWriteUsers, fmt.Errorf("failed to close file %q: %w", tmpFileName, err))
	}

	if err = os.Rename(tmpFileName, s.filename); err != nil {
		return errors.Join(ErrWriteUsers, fmt.Errorf("failed to rename file %q to %q: %w", tmpFileName, s.filename, err))
	}

	return nil
}

// Add creates a new user.
func (s *UserStore) Add(username, salt, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return errors.Join(ErrInconsistentRequest, fmt.Errorf("user already exists: %s", username))
	}

	s.users[username] = User{Username: username, Salt: salt, Hash: passwordHash}
	return s.save()
}

// Delete removes a user by username.
func (s *UserStore) Delete(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; !exists {
		return errors.Join(ErrUserNotFound, fmt.Errorf("username: %s", username))
	}

	delete(s.users, username)
	return s.save()
}

// Update changes user's password and salt.
func (s *UserStore) Update(username, salt, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[username]
	if !exists {
		return errors.Join(ErrUserNotFound, fmt.Errorf("username: %s", username))
	}

	user.Salt = salt
	user.Hash = hash
	s.users[username] = user

	return s.save()
}

// Get returns a user by username.
func (s *UserStore) Get(username string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[username]
	if !exists {
		return User{}, errors.Join(ErrUserNotFound, fmt.Errorf("username: %s", username))
	}

	return user, nil
}

func (s *UserStore) sortedUsers() []User {
	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})

	return users
}

// List returns a sorted list of users.
func (s *UserStore) List() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sortedUsers()
}
