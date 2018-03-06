package stake

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crypto "github.com/tendermint/go-crypto"

	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/state"
)

//______________________________________________________________________

type testCoinSender struct {
	store map[string]int64
}

var _ coinSend = testCoinSender{} // enforce interface at compile time

func (c testCoinSender) transferFn(sender, receiver sdk.Actor, coins coin.Coins) error {
	c.store[string(sender.Address)] -= int64(coins[0].Amount)
	c.store[string(receiver.Address)] += int64(coins[0].Amount)
	return nil
}

//______________________________________________________________________

func initAccounts(n int, amount int64) ([]sdk.Actor, map[string]int64) {
	accStore := map[string]int64{}
	senders := newActors(n)
	for _, sender := range senders {
		accStore[string(sender.Address)] = amount
	}
	return senders, accStore
}

func newTxDeclareCandidacy(amt int64, pubKey crypto.PubKey) TxDeclareCandidacy {
	return TxDeclareCandidacy{
		BondUpdate{
			PubKey: pubKey,
			Bond:   coin.Coin{"fermion", amt},
		},
		Description{},
	}
}

func newTxDelegate(amt int64, pubKey crypto.PubKey) TxDelegate {
	return TxDelegate{BondUpdate{
		PubKey: pubKey,
		Bond:   coin.Coin{"fermion", amt},
	}}
}

func newTxUnbond(shares uint64, pubKey crypto.PubKey) TxUnbond {
	return TxUnbond{
		PubKey: pubKey,
		Shares: shares,
	}
}

func newDeliver(sender sdk.Actor, accStore map[string]int64) deliver {
	store := state.NewMemKVStore()
	return deliver{
		store:    store,
		sender:   sender,
		params:   loadParams(store),
		transfer: testCoinSender{accStore}.transferFn,
	}
}

func newPubKey(pk string) crypto.PubKey {
	pkBytes, _ := hex.DecodeString(pk)
	var pkEd crypto.PubKeyEd25519
	copy(pkEd[:], pkBytes[:])
	return pkEd.Wrap()
}

//dummy public keys used for testing
var (
	pk1 = newPubKey("0B485CFC0EECC619440448436F8FC9DF40566F2369E72400281454CB552AFB57")
	pk2 = newPubKey("E2CB355FD7965D70627AB279016D713CAF82612216D833372187520E264C3588")
	pk3 = newPubKey("03E6F86BC3B9BDF9E6FEAB434BE1A3A2817BF4AE15CE0D4054C6EA7BFC2A30BE")
)

func TestDuplicatesTxDeclareCandidacy(t *testing.T) {
	assert := assert.New(t)
	senders, accStore := initAccounts(2, 1000) // for accounts

	deliverer := newDeliver(senders[0], accStore)
	checker := check{
		store:  deliverer.store,
		sender: senders[0],
	}

	txDeclareCandidacy := newTxDeclareCandidacy(10, pk1)
	got := deliverer.declareCandidacy(txDeclareCandidacy)
	assert.NoError(got, "expected no error on runTxDeclareCandidacy")

	// one sender can bond to two different pubKeys
	txDeclareCandidacy.PubKey = pk2
	err := checker.declareCandidacy(txDeclareCandidacy)
	assert.Nil(err, "didn't expected error on checkTx")

	// two senders cant bond to the same pubkey
	checker.sender = senders[1]
	txDeclareCandidacy.PubKey = pk1
	err = checker.declareCandidacy(txDeclareCandidacy)
	assert.NotNil(err, "expected error on checkTx")
}

func TestIncrementsTxDelegate(t *testing.T) {
	assert := assert.New(t)
	initSender := int64(1000)
	senders, accStore := initAccounts(1, initSender) // for accounts
	deliverer := newDeliver(senders[0], accStore)

	// first declare candidacy
	bondAmount := int64(10)
	txDeclareCandidacy := newTxDeclareCandidacy(bondAmount, pk1)
	got := deliverer.declareCandidacy(txDeclareCandidacy)
	assert.NoError(got, "expected declare candidacy tx to be ok, got %v", got)
	expectedBond := bondAmount // 1 since we send 1 at the start of loop,

	// just send the same txbond multiple times
	holder := deliverer.params.HoldAccount
	txDelegate := newTxDelegate(bondAmount, pk1)
	for i := 0; i < 5; i++ {
		got := deliverer.delegate(txDelegate)
		assert.NoError(got, "expected tx %d to be ok, got %v", i, got)

		//Check that the accounts and the bond account have the appropriate values
		candidates := loadCandidates(deliverer.store)
		expectedBond += bondAmount
		expectedSender := initSender - expectedBond
		gotBonded := int64(candidates[0].Shares)
		gotHolder := accStore[string(holder.Address)]
		gotSender := accStore[string(deliverer.sender.Address)]
		assert.Equal(expectedBond, gotBonded, "%v, %v", expectedBond, gotBonded)
		assert.Equal(expectedBond, gotHolder, "%v, %v", expectedBond, gotHolder)
		assert.Equal(expectedSender, gotSender, "%v, %v", expectedSender, gotSender)
	}
}

func TestIncrementsTxUnbond(t *testing.T) {
	assert := assert.New(t)
	initSender := int64(0)
	senders, accStore := initAccounts(1, initSender) // for accounts
	deliverer := newDeliver(senders[0], accStore)

	// set initial bond
	initBond := int64(1000)
	accStore[string(deliverer.sender.Address)] = initBond
	got := deliverer.declareCandidacy(newTxDeclareCandidacy(initBond, pk1))
	assert.NoError(got, "expected initial bond tx to be ok, got %v", got)

	// just send the same txunbond multiple times
	holder := deliverer.params.HoldAccount
	unbondAmount := uint64(10)
	txUndelegate := newTxUnbond(unbondAmount, pk1)
	nUnbonds := 5
	for i := 0; i < nUnbonds; i++ {
		got := deliverer.unbond(txUndelegate)
		assert.NoError(got, "expected tx %d to be ok, got %v", i, got)

		//Check that the accounts and the bond account have the appropriate values
		candidates := loadCandidates(deliverer.store)
		expectedBond := initBond - int64(i+1)*int64(unbondAmount) // +1 since we send 1 at the start of loop
		expectedSender := initSender + (initBond - expectedBond)
		gotBonded := int64(candidates[0].Shares)
		gotHolder := accStore[string(holder.Address)]
		gotSender := accStore[string(deliverer.sender.Address)]

		assert.Equal(expectedBond, gotBonded, "%v, %v", expectedBond, gotBonded)
		assert.Equal(expectedBond, gotHolder, "%v, %v", expectedBond, gotHolder)
		assert.Equal(expectedSender, gotSender, "%v, %v", expectedSender, gotSender)
	}

	// these are more than we have bonded now
	errorCases := []uint64{
		1<<64 - 1, // more than int64
		1<<63 + 1, // more than int64
		1<<63 - 1,
		1 << 31,
		uint64(initBond),
	}
	for _, c := range errorCases {
		unbondAmount := c
		txUndelegate := newTxUnbond(unbondAmount, pk1)
		got = deliverer.unbond(txUndelegate)
		assert.Error(got, "expected unbond tx to fail")
	}

	leftBonded := uint64(initBond - int64(unbondAmount)*int64(nUnbonds))

	// should be unable to unbond one more than we have
	txUndelegate = newTxUnbond(leftBonded+1, pk1)
	got = deliverer.unbond(txUndelegate)
	assert.Error(got, "expected unbond tx to fail")

	// should be able to unbond just what we have
	txUndelegate = newTxUnbond(leftBonded, pk1)
	got = deliverer.unbond(txUndelegate)
	assert.NoError(got, "expected unbond tx to pass")
}

func TestMultipleTxDeclareCandidacy(t *testing.T) {
	assert := assert.New(t)
	initSender := int64(1000)
	senders, accStore := initAccounts(3, initSender)
	pubKeys := []crypto.PubKey{pk1, pk2, pk3}
	deliverer := newDeliver(senders[0], accStore)

	// bond them all
	for i, sender := range senders {
		txDeclareCandidacy := newTxDeclareCandidacy(10, pubKeys[i])
		deliverer.sender = sender
		got := deliverer.declareCandidacy(txDeclareCandidacy)
		assert.NoError(got, "expected tx %d to be ok, got %v", i, got)

		//Check that the account is bonded
		candidates := loadCandidates(deliverer.store)
		val := candidates[i]
		balanceGot, balanceExpd := accStore[string(val.Owner.Address)], initSender-10
		assert.Equal(i+1, len(candidates), "expected %d candidates got %d, candidates: %v", i+1, len(candidates), candidates)
		assert.Equal(10, int(val.Shares), "expected %d shares, got %d", 10, val.Shares)
		assert.Equal(balanceExpd, balanceGot, "expected account to have %d, got %d", balanceExpd, balanceGot)
	}

	// unbond them all
	for i, sender := range senders {
		candidatePre := loadCandidate(deliverer.store, pubKeys[i])
		txUndelegate := newTxUnbond(10, pubKeys[i])
		deliverer.sender = sender
		got := deliverer.unbond(txUndelegate)
		assert.NoError(got, "expected tx %d to be ok, got %v", i, got)

		//Check that the account is unbonded
		candidates := loadCandidates(deliverer.store)
		assert.Equal(len(senders)-(i+1), len(candidates), "expected %d candidates got %d", len(senders)-(i+1), len(candidates))

		candidatePost := loadCandidate(deliverer.store, pubKeys[i])
		balanceGot, balanceExpd := accStore[string(candidatePre.Owner.Address)], initSender
		assert.Nil(candidatePost, "expected nil candidate retrieve, got %d", 0, candidatePost)
		assert.Equal(balanceExpd, balanceGot, "expected account to have %d, got %d", balanceExpd, balanceGot)
	}
}

func TestMultipleTxDelegate(t *testing.T) {
	assert, require := assert.New(t), require.New(t)
	accounts, accStore := initAccounts(3, 1000)
	sender, delegators := accounts[0], accounts[1:]
	deliverer := newDeliver(sender, accStore)

	//first make a candidate
	txDeclareCandidacy := newTxDeclareCandidacy(10, pk1)
	got := deliverer.declareCandidacy(txDeclareCandidacy)
	require.NoError(got, "expected tx to be ok, got %v", got)

	// delegate multiple parties
	for i, delegator := range delegators {
		txDelegate := newTxDelegate(10, pk1)
		deliverer.sender = delegator
		got := deliverer.delegate(txDelegate)
		require.NoError(got, "expected tx %d to be ok, got %v", i, got)

		//Check that the account is bonded
		bond := loadDelegatorBond(deliverer.store, delegator, pk1)
		assert.NotNil(bond, "expected delegatee bond %d to exist", bond)
	}

	// unbond them all
	for i, delegator := range delegators {
		txUndelegate := newTxUnbond(10, pk1)
		deliverer.sender = delegator
		got := deliverer.unbond(txUndelegate)
		require.NoError(got, "expected tx %d to be ok, got %v", i, got)

		//Check that the account is unbonded
		bond := loadDelegatorBond(deliverer.store, delegator, pk1)
		assert.Nil(bond, "expected delegatee bond %d to be nil", bond)
	}
}

func TestVoidCandidacy(t *testing.T) {
	assert, require := assert.New(t), require.New(t)
	accounts, accStore := initAccounts(2, 1000) // for accounts
	sender, delegator := accounts[0], accounts[1]
	deliverer := newDeliver(sender, accStore)

	// create the candidate
	txDeclareCandidacy := newTxDeclareCandidacy(10, pk1)
	got := deliverer.declareCandidacy(txDeclareCandidacy)
	require.NoError(got, "expected no error on runTxDeclareCandidacy")

	// bond a delegator
	txDelegate := newTxDelegate(10, pk1)
	deliverer.sender = delegator
	got = deliverer.delegate(txDelegate)
	require.NoError(got, "expected ok, got %v", got)

	// unbond the candidates bond portion
	txUndelegate := newTxUnbond(10, pk1)
	deliverer.sender = sender
	got = deliverer.unbond(txUndelegate)
	require.NoError(got, "expected no error on runTxDeclareCandidacy")

	// test that this pubkey cannot yet be bonded too
	deliverer.sender = delegator
	got = deliverer.delegate(txDelegate)
	assert.Error(got, "expected error, got %v", got)

	// test that the delegator can still withdraw their bonds
	got = deliverer.unbond(txUndelegate)
	require.NoError(got, "expected no error on runTxDeclareCandidacy")

	// verify that the pubkey can now be reused
	got = deliverer.declareCandidacy(txDeclareCandidacy)
	assert.NoError(got, "expected ok, got %v", got)

}

// newTxDefineService - new TxDefineService
func newTxDefineService(name string, description string) TxDefineService {
	return TxDefineService{
		Name:          name,
		Description:   description,
	}
}

func TestTxDefineService(t *testing.T) {
	require := assert.New(t)
	accounts, accStore := initAccounts(3, 1000)
	sender := accounts[0]
	deliverer := newDeliver(sender, accStore)

	//first make a candidate
	txDefineService := newTxDefineService("testname", "testdesc")
	deliverer.sender = sender
	got := deliverer.defineService(txDefineService)
	require.NoError(got, "expected tx to be ok, got %v", got)
}