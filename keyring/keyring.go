package keyring

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"

	"cosmossdk.io/math"
	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/app/encoding"
	"github.com/cmwaters/apollo/genesis"
	"github.com/cmwaters/apollo/node/bridge"
	"github.com/cmwaters/apollo/node/consensus"
	"github.com/cmwaters/apollo/node/light"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	KeyringServiceName = "apollo-keyring"
)

var (
	cdc = encoding.MakeConfig(app.ModuleEncodingRegisters...)
)

type Service struct {
	Keyring keyring.Keyring
	idx     uint32
	Records map[string]Key
}

type Key struct {
	Record   *keyring.Record
	Mnemonic string
}

func New() *Service {
	return &Service{
		Records: make(map[string]Key),
	}
}

type unsafeKeyring interface {
	// ExportPrivateKeyObject returns a private key in unarmored format.
	ExportPrivateKeyObject(uid string) (types.PrivKey, error)
}

// unsafeExportPrivKeyHex exports private keys in unarmored hexadecimal format.
func (s *Service) UnsafeExportPrivKeyHex(uid string) (privkey string, err error) {
	priv, err := s.Keyring.(unsafeKeyring).ExportPrivateKeyObject(uid)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(priv.Bytes()), nil
}

func (s *Service) Setup(ctx context.Context, dir string) (genesis.Modifier, error) {
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create directory for keyring: %w", err)
	}

	kr, err := keyring.New(KeyringServiceName, keyring.BackendTest, dir, nil, cdc.Codec)
	if err != nil {
		return nil, err
	}

	s.Keyring = kr

	records, err := kr.List()
	if err != nil {
		return nil, err
	}
	var modifier genesis.Modifier
	if len(records) == 0 {
		// create a key if it doesn't yet exist
		consensusKey, mnemonic, err := s.GenerateKey(consensus.ConsensusServiceName)
		if err != nil {
			return nil, err
		}
		s.Records[consensus.ConsensusServiceName] = Key{Record: consensusKey, Mnemonic: mnemonic}

		bridgeKey, mnemonic, err := s.GenerateKey(bridge.BridgeServiceName)
		if err != nil {
			return nil, err
		}
		s.Records[bridge.BridgeServiceName] = Key{Record: bridgeKey, Mnemonic: mnemonic}

		lightKey, mnemonic, err := s.GenerateKey(light.LightServiceName)
		if err != nil {
			return nil, err
		}
		s.Records[light.LightServiceName] = Key{Record: lightKey, Mnemonic: mnemonic}

		conAddr, err := consensusKey.GetAddress()
		if err != nil {
			return nil, err
		}
		bridgeAddr, err := bridgeKey.GetAddress()
		if err != nil {
			return nil, err
		}
		lightAddr, err := lightKey.GetAddress()
		if err != nil {
			return nil, err
		}

		modifier = genesis.FundAccounts(cdc.Codec, []sdk.AccAddress{conAddr, bridgeAddr, lightAddr}, sdk.NewCoin(app.DisplayDenom, math.NewInt(10_000_000)))
	} else {
		_, err := kr.Key(KeyringServiceName)
		if err != nil {
			return nil, err
		}
		modifier = nil
	}

	return modifier, nil
}

func (s *Service) GenerateKey(name string) (*keyring.Record, string, error) {
	if s.Keyring == nil {
		return nil, "", fmt.Errorf("keyring not yet initialized. please call Setup first")
	}
	path := hd.CreateHDPath(sdk.CoinType, 0, s.idx)
	s.idx++
	return s.Keyring.NewMnemonic(name, keyring.English, keyring.DefaultBIP39Passphrase, path.String(), hd.Secp256k1)
}
