package consensus

import (
	"encoding/json"

	"github.com/celestiaorg/apollo/genesis"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
)

// AddValidator creates a genesis modifier for adding a validator.
// It funds the account in the auth and bank state and adds the signed gen tx for creating that validator.
func AddValidator(pubKey cryptotypes.PubKey, coins sdk.Coins, genTx json.RawMessage) genesis.Modifier {
	return func(state map[string]json.RawMessage) map[string]json.RawMessage {
		address := sdk.AccAddress(pubKey.Address())

		// Fund the account in the auth state
		var authState authtypes.GenesisState
		cdc.Codec.MustUnmarshalJSON(state[authtypes.ModuleName], &authState)

		account := authtypes.NewBaseAccount(address, pubKey, uint64(len(authState.Accounts)), 0)
		genAccounts, err := authtypes.PackAccounts([]authtypes.GenesisAccount{account})
		if err != nil {
			panic(err)
		}
		authState.Accounts = append(authState.Accounts, genAccounts...)
		state[authtypes.ModuleName] = cdc.Codec.MustMarshalJSON(&authState)

		// Fund the account in the bank state
		var bankState banktypes.GenesisState
		cdc.Codec.MustUnmarshalJSON(state[banktypes.ModuleName], &bankState)
		balance := banktypes.Balance{Address: address.String(), Coins: coins}
		bankState.Balances = append(bankState.Balances, balance)
		state[banktypes.ModuleName] = cdc.Codec.MustMarshalJSON(&bankState)

		// Add the signed gen tx for creating the validator
		var genutilState genutiltypes.GenesisState
		cdc.Codec.MustUnmarshalJSON(state[genutiltypes.ModuleName], &genutilState)
		genutilState.GenTxs = append(genutilState.GenTxs, genTx)
		state[genutiltypes.ModuleName] = cdc.Codec.MustMarshalJSON(&genutilState)

		return state
	}
}
