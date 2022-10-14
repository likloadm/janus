package qtum

import (
	"encoding/hex"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
)

type Accounts []*btcutil.WIF

func (as Accounts) FindByHexAddress(addr string) *btcutil.WIF {
	for _, a := range as {
		acc := &Account{a}

		if addr == acc.ToHexAddress() {
			return a
		}
	}

	return nil
}

type Account struct {
	*btcutil.WIF
}

func (a *Account) ToHexAddress() string {
	// wif := (*btcutil.WIF)(a)

	keyid := btcutil.Hash160(a.SerializePubKey())
	return hex.EncodeToString(keyid)
}

var qtumMainNetParams = chaincfg.MainNetParams
var qtumTestNetParams = chaincfg.MainNetParams

func init() {
	qtumMainNetParams.PubKeyHashAddrID = 58
	qtumMainNetParams.ScriptHashAddrID = 23

	qtumTestNetParams.PubKeyHashAddrID = 120
	qtumTestNetParams.ScriptHashAddrID = 110
}

func (a *Account) ToBase58Address(isMain bool) (string, error) {
	params := &qtumMainNetParams
	if !isMain {
		params = &qtumTestNetParams
	}

	addr, err := btcutil.NewAddressPubKey(a.SerializePubKey(), params)
	if err != nil {
		return "", err
	}

	return addr.AddressPubKeyHash().String(), nil
}
