package faucet

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/dgraph-io/badger/v3"
)

type Limit struct {
	Window time.Duration `toml:"window"`
	Amount uint64        `toml:"amount"`
}

type Config struct {
	InitialSupply   uint64 `toml:"initial_supply"`
	Amount          uint64 `toml:"amount"`
	PerAccountLimit Limit  `toml:"per_account_limit"`
	GlobalLimit     Limit  `toml:"global_limit"`
	APIAddress      string `toml:"api_address"`
	GUIAddress      string `toml:"gui_address"`
}

func DefaultConfig() *Config {
	return &Config{
		InitialSupply: 1_000_000_000_000, // 1 million TIA
		Amount:        10_000_000,        // 10 TIA
		PerAccountLimit: Limit{
			Amount: 10_000_000, // 10 TIA
			Window: time.Hour,
		},
		APIAddress: "localhost:1095",
	}
}

type Store struct {
	db     *badger.DB
	config *Config
}

func NewStore(dbPath string, config *Config) (*Store, error) {
	opts := badger.DefaultOptions(dbPath).WithLoggingLevel(badger.ERROR)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, config: config}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) addTimestampForAddress(address types.AccAddress, timestamp time.Time) error {
	return s.db.Update(func(txn *badger.Txn) error {
		var timestamps []time.Time
		item, err := txn.Get(address.Bytes())
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		if err == nil {
			err = item.Value(func(val []byte) error {
				return json.Unmarshal(val, &timestamps)
			})
			if err != nil {
				return err
			}
		}
		timestamps = append(timestamps, timestamp)
		timestampsBytes, err := json.Marshal(timestamps)
		if err != nil {
			return err
		}
		return txn.Set(address.Bytes(), timestampsBytes)
	})
}
func (s *Store) getTimestampsForAddress(address types.AccAddress) ([]time.Time, error) {
	var timestamps []time.Time
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(address.Bytes())
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &timestamps)
		})
	})
	return timestamps, err
}

func (s *Store) RequestFunds(address types.AccAddress) error {
	timestamps, err := s.getTimestampsForAddress(address)
	if err != nil {
		return err
	}

	// Check if the rate limit has been exceeded
	now := time.Now()
	var recentRequests uint64
	for _, timestamp := range timestamps {
		if now.Sub(timestamp) <= s.config.PerAccountLimit.Window {
			recentRequests++
		}

		// Avoid double requests
		if now.Sub(timestamp) < 10*time.Second {
			return errors.New("request has come too soon after the previous request")
		}
	}

	if recentRequests*s.config.Amount >= s.config.PerAccountLimit.Amount {
		waitTime := s.config.PerAccountLimit.Window - time.Since(timestamps[len(timestamps)-1])
		return fmt.Errorf("rate limit exceeded, please wait %s to request funds again", waitTime)
	}

	// TODO: the global rate limit is not yet enforced

	// Add a new timestamp for this request
	err = s.addTimestampForAddress(address, now)
	if err != nil {
		return err
	}

	return nil
}
