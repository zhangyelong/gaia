package stake

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/modules/coin"

	crypto "github.com/tendermint/go-crypto"
	wire "github.com/tendermint/go-wire"
)

var (
	validator = sdk.Actor{"testChain", "testapp", []byte("addressvalidator1")}
	empty     sdk.Actor

	coinPos          = coin.Coin{"fermion", 1000}
	coinZero         = coin.Coin{"fermion", 0}
	coinNeg          = coin.Coin{"fermion", -10000}
	coinPosNotAtoms  = coin.Coin{"foo", 10000}
	coinZeroNotAtoms = coin.Coin{"foo", 0}
	coinNegNotAtoms  = coin.Coin{"foo", -10000}
)

func TestBondUpdateValidateBasic(t *testing.T) {
	type fields struct {
		PubKey crypto.PubKey
		Bond   coin.Coin
	}

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{"basic good", fields{pk1, coinPos}, false},
		{"empty delegator", fields{crypto.PubKey{}, coinPos}, true},
		{"zero coin", fields{pk1, coinZero}, true},
		{"neg coin", fields{pk1, coinNeg}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := TxDelegate{BondUpdate{
				PubKey: tt.fields.PubKey,
				Bond:   tt.fields.Bond,
			}}
			assert.Equal(t, tt.wantErr, tx.ValidateBasic() != nil,
				"test: %v, tx.ValidateBasic: %v", tt.name, tx.ValidateBasic())
		})
	}
}

func TestAllAreTx(t *testing.T) {
	assert := assert.New(t)

	// make sure all types construct properly
	//pubKey := newPubKey("1234567890")
	//bondAmt := uint64(1234321)
	//bond := coin.Coin{Denom: "ATOM", Amount: int64(bondAmt)}

	// Note that Wrap is only defined on BondUpdate, so when you call it,
	// you lose all info on the embedding type. Please add Wrap()
	// method to all the parents
	//txDelegate := NewTxDelegate(bond, pubKey)
	//_, ok := txDelegate.Unwrap().(TxDelegate)
	//assert.True(ok, "%#v", txDelegate)
	//
	//txUnbond := NewTxUnbond(bondAmt, pubKey)
	//_, ok = txUnbond.Unwrap().(TxUnbond)
	//assert.True(ok, "%#v", txUnbond)
	//
	//txDecl := NewTxDeclareCandidacy(bond, pubKey, Description{})
	//_, ok = txDecl.Unwrap().(TxDeclareCandidacy)
	//assert.True(ok, "%#v", txDecl)
	//
	//txEditCan := NewTxEditCandidacy(pubKey, Description{})
	//_, ok = txEditCan.Unwrap().(TxEditCandidacy)
	//assert.True(ok, "%#v", txEditCan)

	txDefineSvc := NewTxDefineService("", "")
	_, ok := txDefineSvc.Unwrap().(TxDefineService)
	assert.True(ok, "%#v", txDefineSvc)


}

func TestSerializeTx(t *testing.T) {
	assert := assert.New(t)

	// make sure all types construct properly
	pubKey := newPubKey("1234567890")
	bondAmt := uint64(1234321)
	bond := coin.Coin{Denom: "ATOM", Amount: int64(bondAmt)}

	cases := []struct {
		tx sdk.Tx
	}{
		{NewTxUnbond(bondAmt, pubKey)},
		{NewTxDeclareCandidacy(bond, pubKey, Description{})},
		{NewTxDeclareCandidacy(bond, pubKey, Description{})},
		// {NewTxRevokeCandidacy(pubKey)},
	}

	for i, tc := range cases {
		var tx sdk.Tx
		bs := wire.BinaryBytes(tc.tx)
		err := wire.ReadBinaryBytes(bs, &tx)
		if assert.NoError(err, "%d", i) {
			assert.Equal(tc.tx, tx, "%d", i)
		}
	}
}
