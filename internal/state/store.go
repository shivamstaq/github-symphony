package state

import (
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketRetries = []byte("retries")
	bucketTotals  = []byte("totals")
	keyTotals     = []byte("agent_totals")
)

// RetryRecord is a persistent retry entry.
type RetryRecord struct {
	WorkItemID      string `json:"work_item_id"`
	ProjectItemID   string `json:"project_item_id,omitempty"`
	IssueIdentifier string `json:"issue_identifier,omitempty"`
	Attempt         int    `json:"attempt"`
	DueAtMs         int64  `json:"due_at_ms"`
	Error           string `json:"error,omitempty"`
}

// AgentTotalsRecord persists aggregate counters.
type AgentTotalsRecord struct {
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	SecondsRunning   float64 `json:"seconds_running"`
	GitHubWritebacks int64   `json:"github_writebacks"`
	SessionsStarted  int64   `json:"sessions_started"`
}

// Store is the bbolt-backed persistent state.
type Store struct {
	db *bolt.DB
}

// Open creates or opens the state database.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("state store open: %w", err)
	}

	// Ensure buckets exist
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketRetries); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketTotals); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state store init: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveRetry persists a retry entry.
func (s *Store) SaveRetry(r RetryRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRetries)
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		return b.Put([]byte(r.WorkItemID), data)
	})
}

// DeleteRetry removes a retry entry.
func (s *Store) DeleteRetry(workItemID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketRetries).Delete([]byte(workItemID))
	})
}

// LoadRetries returns all persisted retry entries.
func (s *Store) LoadRetries() ([]RetryRecord, error) {
	var records []RetryRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRetries)
		return b.ForEach(func(_, v []byte) error {
			var r RetryRecord
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			records = append(records, r)
			return nil
		})
	})
	return records, err
}

// SaveTotals persists aggregate counters.
func (s *Store) SaveTotals(t AgentTotalsRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTotals)
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		return b.Put(keyTotals, data)
	})
}

// LoadTotals returns the persisted aggregate counters.
func (s *Store) LoadTotals() (AgentTotalsRecord, error) {
	var t AgentTotalsRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTotals)
		data := b.Get(keyTotals)
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &t)
	})
	return t, err
}
