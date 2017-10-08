package main

import (
	"os"

	"github.com/spf13/cobra"

	abci "github.com/tendermint/abci/types"
	"github.com/tendermint/tmlibs/cli"

	client "github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/modules/auth"
	"github.com/cosmos/cosmos-sdk/modules/base"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/modules/fee"
	"github.com/cosmos/cosmos-sdk/modules/ibc"
	"github.com/cosmos/cosmos-sdk/modules/nonce"
	"github.com/cosmos/cosmos-sdk/modules/roles"
	basecmd "github.com/cosmos/cosmos-sdk/server/commands"
	"github.com/cosmos/cosmos-sdk/stack"
	"github.com/cosmos/cosmos-sdk/state"
	"github.com/cosmos/gaia/modules/stake"
)

// RootCmd is the entry point for this binary
var RootCmd = &cobra.Command{
	Use:   "gaia",
	Short: "The Cosmos Network delegation-game blockchain test",
}

// Tick - Called every block even if no transaction,
//   process all queues, validator rewards, and calculate the validator set difference
func getTickFnc(updateVal bool) func(store state.SimpleDB) (diffVal []*abci.Validator, err error) {
	return func(store state.SimpleDB) (diffVal []*abci.Validator, err error) {

		// First need to prefix the store, at this point it's a global store
		store = stack.PrefixedStore(stake.Name(), store)

		// Determine the validator set changes
		validatorBonds, err := stake.LoadBonds(store)
		if err != nil {
			panic(err) //This error should really never happen - vital that this loads properly
		}
		if updateVal {
			startVal := validatorBonds.GetValidators()
			validatorBonds.UpdateVotingPower(store)
			newVal := validatorBonds.GetValidators()
			diffVal = stake.ValidatorsDiff(startVal, newVal)
		} else {
			validatorBonds.UpdateVotingPower(store)
		}

		return
	}
}

func main() {
	// require all fees in mycoin - change this in your app!
	basecmd.Handler = stack.New(
		base.Logger{},
		stack.Recovery{},
		auth.Signatures{},
		base.Chain{},
		stack.Checkpoint{OnCheck: true},
		nonce.ReplayCheck{},
	).
		IBC(ibc.NewMiddleware()).
		Apps(
			roles.NewMiddleware(),
			fee.NewSimpleFeeMiddleware(coin.Coin{"mycoin", 0}, fee.Bank),
			stack.Checkpoint{OnDeliver: true},
		).
		Dispatch(
			coin.NewHandler(),
			stack.WrapHandler(roles.NewHandler()),
			stack.WrapHandler(ibc.NewHandler()),
			stake.NewHandler(),
		)

	var startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start this full node",
	}
	basecmd.InitTickStartCmd(getTickFnc(true), startCmd)

	var startNoValCmd = &cobra.Command{
		Use:   "startnoval",
		Short: "Start a full node that never updates the validator set with tendermint",
	}
	basecmd.InitTickStartCmd(getTickFnc(false), startNoValCmd)

	RootCmd.AddCommand(
		basecmd.InitCmd,
		startCmd,
		startNoValCmd,
		basecmd.UnsafeResetAllCmd,
		client.VersionCmd,
	)
	basecmd.SetUpRoot(RootCmd)

	cmd := cli.PrepareMainCmd(RootCmd, "GA", os.ExpandEnv("$HOME/.cosmos-gaia"))
	cmd.Execute()
}
