package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUser(t *testing.T) {
	tests := []struct {
		name     string
		username string
		salt     string
		hash     string
		wantErr  error
	}{
		{
			name:     "valid user",
			username: "test-user",
			salt:     "test-salt",
			hash:     "test-hash",
			wantErr:  nil,
		},
		{
			name:     "empty username",
			username: "",
			salt:     "test-salt",
			hash:     "test-hash",
			wantErr:  ErrInconsistentRequest,
		},
		{
			name:     "empty salt",
			username: "test-user",
			salt:     "",
			hash:     "test-hash",
			wantErr:  ErrInconsistentRequest,
		},
		{
			name:     "empty hash",
			username: "test-user",
			salt:     "test-salt",
			hash:     "",
			wantErr:  ErrInconsistentRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := NewUser(tt.username, tt.salt, tt.hash)

			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr))
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.username, user.Username)
				assert.Equal(t, tt.salt, user.Salt)
				assert.Equal(t, tt.hash, user.Hash)
			}
		})
	}
}

func TestUserStore(t *testing.T) {
	tmpDir := t.TempDir()

	filename := filepath.Join(tmpDir, "users.csv")
	store, errStorage := NewStorage(filename)
	require.NoError(t, errStorage)

	t.Run("Add", func(t *testing.T) {
		err := store.Add("user1", "salt1", "hash1")
		assert.NoError(t, err)

		err = store.Add("user1", "salt2", "hash2")
		assert.True(t, errors.Is(err, ErrInconsistentRequest))

		err = store.Add("user2", "salt2", "hash2")
		assert.NoError(t, err)
	})

	t.Run("Get", func(t *testing.T) {
		user, err := store.Get("user1")
		assert.NoError(t, err)

		assert.Equal(t, "user1", user.Username)
		assert.Equal(t, "salt1", user.Salt)
		assert.Equal(t, "hash1", user.Hash)

		_, err = store.Get("nonexistent")
		assert.True(t, errors.Is(err, ErrUserNotFound))
	})

	t.Run("Update", func(t *testing.T) {
		err := store.Update("user1", "new-salt", "new-hash")
		assert.NoError(t, err)

		user, err := store.Get("user1")
		assert.NoError(t, err)
		assert.Equal(t, "new-salt", user.Salt)
		assert.Equal(t, "new-hash", user.Hash)

		err = store.Update("nonexistent", "salt", "hash")
		assert.True(t, errors.Is(err, ErrUserNotFound))
	})

	t.Run("Delete", func(t *testing.T) {
		err := store.Delete("user1")
		assert.NoError(t, err)

		_, err = store.Get("user1")
		assert.True(t, errors.Is(err, ErrUserNotFound))

		err = store.Delete("nonexistent")
		assert.True(t, errors.Is(err, ErrUserNotFound))
	})

	t.Run("List", func(t *testing.T) {
		require.NoError(t, store.Add("charlie", "salt3", "hash3"))
		require.NoError(t, store.Add("alice", "salt1", "hash1"))
		require.NoError(t, store.Add("bob", "salt2", "hash2"))

		// sorted list
		users := store.List()
		assert.Equal(t, 4, len(users)) // including user2 from previous tests
		assert.Equal(t, "alice", users[0].Username)
		assert.Equal(t, "bob", users[1].Username)
		assert.Equal(t, "charlie", users[2].Username)
		assert.Equal(t, "user2", users[3].Username)
	})
}

func TestStoragePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "users.csv")

	store1, err := NewStorage(filename)
	require.NoError(t, err)
	require.NoError(t, store1.Add("user1", "salt1", "hash1"))
	require.NoError(t, store1.Add("user2", "salt2", "hash2"))

	// reading the same file
	store2, err := NewStorage(filename)
	require.NoError(t, err)

	users := store2.List()
	assert.Equal(t, users, []User{
		{Username: "user1", Salt: "salt1", Hash: "hash1"},
		{Username: "user2", Salt: "salt2", Hash: "hash2"},
	})

	user1, err := store2.Get("user1")
	require.NoError(t, err)
	assert.Equal(t, user1.record(), []string{"user1", "salt1", "hash1"})

	user2, err := store2.Get("user2")
	assert.NoError(t, err)
	assert.Equal(t, user2.record(), []string{"user2", "salt2", "hash2"})
}

func TestStorageLoadInvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "users.csv")

	// file with invalid content
	invalidContent := []byte("invalid,csv,file,op\nwith,too,many,fields")
	err := os.WriteFile(filename, invalidContent, 0600)
	require.NoError(t, err)

	store, err := NewStorage(filename)
	require.NoError(t, err) // no errors as invalid records are skipped

	users := store.List()
	assert.Empty(t, users)
}

func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "users.csv")

	store, errStorage := NewStorage(filename)
	require.NoError(t, errStorage)
	require.NoError(t, store.Add("user1", "salt1", "hash1"))

	t.Run("ConcurrentReads", func(t *testing.T) {
		const total = 20
		var wg sync.WaitGroup
		wg.Add(total)

		for range total {
			go func() {
				_, err := store.Get("user1")
				assert.NoError(t, err)
				wg.Done()
			}()
		}
		wg.Wait()
	})

	t.Run("ConcurrentUpdates", func(t *testing.T) {
		const total = 10
		var wg sync.WaitGroup
		wg.Add(total)

		for i := range total {
			go func(i int) {
				var (
					salt = fmt.Sprintf("salt%d", i)
					hash = fmt.Sprintf("hash%d", i)
				)

				assert.NoError(t, store.Update("user1", salt, hash))
				wg.Done()
			}(i)
		}
		wg.Wait()
	})
}
