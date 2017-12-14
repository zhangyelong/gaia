package stake

import (
	"bytes"
	"sort"

	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/state"

	abci "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"
	wire "github.com/tendermint/go-wire"
)

// Params defines the high level settings for staking
type Params struct {
	IssuedGlobalStakeShares int64     `json:"issued_stake_shares"` // sum of all the validators global shares
	TotalSupply             int64     `json:"total_supply"`        // total supply of all tokens
	BondedPool              int64     `json:"bonded_pool"`         // reserve of all bonded tokens
	UnbondedPool            int64     `json:"unbonded_pool"`       // reserve of unbonded tokens held with candidates
	HoldBonded              sdk.Actor `json:"hold_bonded"`         // account  where all bonded coins are held
	HoldUnbonded            sdk.Actor `json:"hold_unbonded"`       // account where all delegated but unbonded coins are held

	Inflation           Fraction `json:"inflation"`             // current annual inflation rate
	InflationRateChange Fraction `json:"inflation_rate_change"` // maximum annual change in inflation rate
	InflationMax        Fraction `json:"inflation_max"`         // maximum inflation rate
	InflationMin        Fraction `json:"inflation_min"`         // minimum inflation rate
	GoalBonded          Fraction `json:"goal_bonded"`           // Goal of percent bonded atoms

	MaxVals          uint16 `json:"max_vals"`           // maximum number of validators
	AllowedBondDenom string `json:"allowed_bond_denom"` // bondable coin denomination

	// gas costs for txs
	GasDeclareCandidacy int64 `json:"gas_declare_candidacy"`
	GasEditCandidacy    int64 `json:"gas_edit_candidacy"`
	GasDelegate         int64 `json:"gas_delegate"`
	GasUnbond           int64 `json:"gas_unbond"`
}

func defaultParams() Params {
	return Params{
		IssuedGlobalStakeShares: 0,
		TotalSupply:             0,
		BondedPool:              0,
		UnbondedPool:            0,
		HoldBonded:              sdk.NewActor(stakingModuleName, []byte("77777777777777777777777777777777")),
		HoldUnbonded:            sdk.NewActor(stakingModuleName, []byte("88888888888888888888888888888888")),
		Inflation:               NewFraction(7, 100),
		InflationRateChange:     NewFraction(13, 100),
		InflationMax:            NewFraction(20, 100),
		InflationMin:            NewFraction(7, 100),
		BondRatioGoal:           NewFraction(67, 100),
		MaxVals:                 100,
		AllowedBondDenom:        "fermion",
		GasDeclareCandidacy:     20,
		GasEditCandidacy:        20,
		GasDelegate:             20,
		GasUnbond:               20,
	}
}

//_________________________________________________________________________

// CandidateStatus - status of a validator-candidate
type CandidateStatus byte

const (
	// nolint
	Active   CandidateStatus = 0x00
	Unbonded CandidateStatus = 0x01
)

// Candidate defines the total amount of bond shares and their exchange rate to
// coins. Accumulation of interest is modelled as an in increase in the
// exchange rate, and slashing as a decrease.  When coins are delegated to this
// candidate, the candidate is credited with a DelegatorBond whose number of
// bond shares is based on the amount of coins delegated divided by the current
// exchange rate. Voting power can be calculated as total bonds multiplied by
// exchange rate.
type Candidate struct {
	Status                CandidateStatus `json:"status"`                  // Bonded status of validator
	PubKey                crypto.PubKey   `json:"pub_key"`                 // Pubkey of candidate
	Owner                 sdk.Actor       `json:"owner"`                   // Sender of BondTx - UnbondTx returns here
	SharesPool            int64           `json:"shares_global_stake"`     // total shares of the glo
	SharesIssuedDelegator int64           `json:"shares_issued_delegator"` // total shares issued to a candidates delegators
	Shares                int64           `json:"shares"`                  // Total number of delegated shares to this candidate
	VotingPower           int64           `json:"voting_power"`            // Voting power if pubKey is a considered a validator
	Description           Description     `json:"description"`             // Description terms for the candidate
}

// Description - description fields for a candidate
type Description struct {
	Moniker  string `json:"moniker"`
	Identity string `json:"identity"`
	Website  string `json:"website"`
	Details  string `json:"details"`
}

// NewCandidate - initialize a new candidate
func NewCandidate(pubKey crypto.PubKey, owner sdk.Actor) *Candidate {
	return &Candidate{
		PubKey:      pubKey,
		Owner:       owner,
		Shares:      0,
		VotingPower: 0,
	}
}

// Validator returns a copy of the Candidate as a Validator.
// Should only be called when the Candidate qualifies as a validator.
func (c *Candidate) validator() Validator {
	return Validator(*c)
}

// Validator is one of the top Candidates
type Validator Candidate

// ABCIValidator - Get the validator from a bond value
func (v Validator) ABCIValidator() *abci.Validator {
	return &abci.Validator{
		PubKey: wire.BinaryBytes(v.PubKey),
		Power:  v.VotingPower,
	}
}

//_________________________________________________________________________

// TODO replace with sorted multistore functionality

// Candidates - list of Candidates
type Candidates []*Candidate

var _ sort.Interface = Candidates{} //enforce the sort interface at compile time

// nolint - sort interface functions
func (cs Candidates) Len() int      { return len(cs) }
func (cs Candidates) Swap(i, j int) { cs[i], cs[j] = cs[j], cs[i] }
func (cs Candidates) Less(i, j int) bool {
	vp1, vp2 := cs[i].VotingPower, cs[j].VotingPower
	pk1, pk2 := cs[i].PubKey.Bytes(), cs[j].PubKey.Bytes()

	//note that all ChainId and App must be the same for a group of candidates
	if vp1 != vp2 {
		return vp1 > vp2
	}
	return bytes.Compare(pk1, pk2) == -1
}

// Sort - Sort the array of bonded values
func (cs Candidates) Sort() {
	sort.Sort(cs)
}

//func updateVotingPower(store state.SimpleDB) {
//candidates := loadCandidates(store)
//candidates.updateVotingPower(store)
//}

// update the voting power and save
func (cs Candidates) updateVotingPower(store state.SimpleDB, params Params) Candidates {

	// update voting power
	for _, c := range cs {
		if c.VotingPower != c.Shares {
			c.VotingPower = c.Shares
		}
	}
	cs.Sort()
	for i, c := range cs {
		// truncate the power
		if i >= int(params.MaxVals) {
			c.VotingPower = 0
		}
		saveCandidate(store, c)
	}
	return cs
}

// Validators - get the most recent updated validator set from the
// Candidates. These bonds are already sorted by VotingPower from
// the UpdateVotingPower function which is the only function which
// is to modify the VotingPower
func (cs Candidates) Validators() Validators {

	//test if empty
	if len(cs) == 1 {
		if cs[0].VotingPower == 0 {
			return nil
		}
	}

	validators := make(Validators, len(cs))
	for i, c := range cs {
		if c.VotingPower == 0 { //exit as soon as the first Voting power set to zero is found
			return validators[:i]
		}
		validators[i] = c.validator()
	}

	return validators
}

//_________________________________________________________________________

// Validators - list of Validators
type Validators []Validator

var _ sort.Interface = Validators{} //enforce the sort interface at compile time

// nolint - sort interface functions
func (vs Validators) Len() int      { return len(vs) }
func (vs Validators) Swap(i, j int) { vs[i], vs[j] = vs[j], vs[i] }
func (vs Validators) Less(i, j int) bool {
	pk1, pk2 := vs[i].PubKey.Bytes(), vs[j].PubKey.Bytes()
	return bytes.Compare(pk1, pk2) == -1
}

// Sort - Sort validators by pubkey
func (vs Validators) Sort() {
	sort.Sort(vs)
}

// determine all changed validators between two validator sets
func (vs Validators) validatorsChanged(vs2 Validators) (changed []*abci.Validator) {

	//first sort the validator sets
	vs.Sort()
	vs2.Sort()

	max := len(vs) + len(vs2)
	changed = make([]*abci.Validator, max)
	i, j, n := 0, 0, 0 //counters for vs loop, vs2 loop, changed element

	for i < len(vs) && j < len(vs2) {

		if !vs[i].PubKey.Equals(vs2[j].PubKey) {
			// pk1 > pk2, a new validator was introduced between these pubkeys
			if bytes.Compare(vs[i].PubKey.Bytes(), vs2[j].PubKey.Bytes()) == 1 {
				changed[n] = vs2[j].ABCIValidator()
				n++
				j++
				continue
			} // else, the old validator has been removed
			changed[n] = &abci.Validator{vs[i].PubKey.Bytes(), 0}
			n++
			i++
			continue
		}

		if vs[i].VotingPower != vs2[j].VotingPower {
			changed[n] = vs2[j].ABCIValidator()
			n++
		}
		j++
		i++
	}

	// add any excess validators in set 2
	for ; j < len(vs2); j, n = j+1, n+1 {
		changed[n] = vs2[j].ABCIValidator()
	}

	// remove any excess validators left in set 1
	for ; i < len(vs); i, n = i+1, n+1 {
		changed[n] = &abci.Validator{vs[i].PubKey.Bytes(), 0}
	}

	return changed[:n]
}

// UpdateValidatorSet - Updates the voting power for the candidate set and
// returns the subset of validators which have changed for Tendermint
func UpdateValidatorSet(store state.SimpleDB, params Params) (change []*abci.Validator, err error) {

	// get the validators before update
	candidates := loadCandidates(store)

	v1 := candidates.Validators()
	v2 := candidates.updateVotingPower(store, params).Validators()

	change = v1.validatorsChanged(v2)
	return
}

//_________________________________________________________________________

// DelegatorBond represents the bond with tokens held by an account.  It is
// owned by one delegator, and is associated with the voting power of one
// pubKey.
type DelegatorBond struct {
	PubKey crypto.PubKey
	Shares int64
}
