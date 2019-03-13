package dposv3

import (
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	loom "github.com/loomnetwork/go-loom"
	common "github.com/loomnetwork/go-loom/common"
	"github.com/loomnetwork/go-loom/plugin"
	"github.com/loomnetwork/go-loom/plugin/contractpb"
	types "github.com/loomnetwork/go-loom/types"
	"github.com/loomnetwork/loomchain/builtin/plugins/coin"

	dtypes "github.com/loomnetwork/go-loom/builtin/types/dposv3"
)

var (
	validatorPubKeyHex1 = "3866f776276246e4f9998aa90632931d89b0d3a5930e804e02299533f55b39e1"
	validatorPubKeyHex2 = "7796b813617b283f81ea1747fbddbe73fe4b5fce0eac0728e47de51d8e506701"
	validatorPubKeyHex3 = "e4008e26428a9bca87465e8de3a8d0e9c37a56ca619d3d6202b0567528786618"

	delegatorAddress1 = loom.MustParseAddress("chain:0xb16a379ec18d4093666f8f38b11a3071c920207d")
	delegatorAddress2 = loom.MustParseAddress("chain:0xfa4c7920accfd66b86f5fd0e69682a79f762d49e")
	delegatorAddress3 = loom.MustParseAddress("chain:0x5cecd1f7261e1f4c684e297be3edf03b825e01c4")
	delegatorAddress4 = loom.MustParseAddress("chain:0x000000000000000000000000e3edf03b825e01e0")
	delegatorAddress5 = loom.MustParseAddress("chain:0x020000000000000000000000e3edf03b825e0288")
)

func TestRegisterWhitelistedCandidate(t *testing.T) {
	oraclePubKey, _ := hex.DecodeString(validatorPubKeyHex2)
	oracleAddr := loom.Address{
		Local: loom.LocalAddressFromPublicKey(oraclePubKey),
	}

	pubKey, _ := hex.DecodeString(validatorPubKeyHex1)
	addr := loom.Address{
		Local: loom.LocalAddressFromPublicKey(pubKey),
	}
	pubKey2, _ := hex.DecodeString(validatorPubKeyHex2)
	addr2 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey2),
	}
	pctx := plugin.CreateFakeContext(addr, addr)

	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(addr2, 2000000000000000000),
		},
	})

	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(oracleAddr)), &InitRequest{
		Params: &Params{
			ValidatorCount: 21,
			OracleAddress:  oracleAddr.MarshalPB(),
		},
	})
	require.NoError(t, err)

	whitelistAmount := loom.BigUInt{big.NewInt(1000000000000)}
	err = dposContract.ProcessRequestBatch(contractpb.WrapPluginContext(dposCtx.WithSender(oracleAddr)), &RequestBatch{
		Batch: []*dtypes.BatchRequest{
			&dtypes.BatchRequest{
				Payload: &dtypes.BatchRequest_WhitelistCandidate{&WhitelistCandidateRequest{
					CandidateAddress: addr.MarshalPB(),
					Amount:           &types.BigUInt{Value: whitelistAmount},
					LockTime:         10,
				}},
				Meta: &dtypes.BatchRequestMeta{
					BlockNumber: 1,
					TxIndex:     0,
					LogIndex:    0,
				},
			},
		},
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr)), &RegisterCandidateRequest{
		PubKey: pubKey,
	})
	require.Nil(t, err)

	err = dposContract.UnregisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr)), &UnregisterCandidateRequest{})
	require.Nil(t, err)

	registrationFee := &types.BigUInt{Value: *scientificNotation(defaultRegistrationRequirement, tokenDecimals)}
	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr2)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr2)), &RegisterCandidateRequest{
		PubKey: pubKey2,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr)), &RegisterCandidateRequest{
		PubKey: pubKey,
	})
	require.Nil(t, err)

	err = dposContract.RemoveWhitelistedCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(oracleAddr)), &RemoveWhitelistedCandidateRequest{
		CandidateAddress: addr.MarshalPB(),
	})
	require.Nil(t, err)

	listResponse, err := dposContract.ListCandidates(contractpb.WrapPluginContext(dposCtx.WithSender(addr)), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, 2, len(listResponse.Candidates))

	err = dposContract.UnregisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr)), &UnregisterCandidateRequest{})
	require.Nil(t, err)

	listResponse, err = dposContract.ListCandidates(contractpb.WrapPluginContext(dposCtx.WithSender(addr)), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, 1, len(listResponse.Candidates))

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr)), &RegisterCandidateRequest{
		PubKey: pubKey,
	})
	require.NotNil(t, err)
}

func TestChangeFee(t *testing.T) {
	oldFee := uint64(100)
	newFee := uint64(1000)
	oraclePubKey, _ := hex.DecodeString(validatorPubKeyHex2)
	oracleAddr := loom.Address{
		Local: loom.LocalAddressFromPublicKey(oraclePubKey),
	}

	dposContract := &DPOS{}

	pubKey, _ := hex.DecodeString(validatorPubKeyHex1)
	addr := loom.Address{
		Local: loom.LocalAddressFromPublicKey(pubKey),
	}
	pctx := plugin.CreateFakeContext(addr, addr)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	_ = pctx.CreateContract(contractpb.MakePluginContract(coinContract))

	err := dposContract.Init(contractpb.WrapPluginContext(pctx.WithSender(oracleAddr)), &InitRequest{
		Params: &Params{
			ValidatorCount: 21,
			OracleAddress:  oracleAddr.MarshalPB(),
		},
	})
	require.Nil(t, err)

	err = dposContract.ProcessRequestBatch(contractpb.WrapPluginContext(pctx.WithSender(oracleAddr)), &RequestBatch{
		Batch: []*dtypes.BatchRequest{
			&dtypes.BatchRequest{
				Payload: &dtypes.BatchRequest_WhitelistCandidate{&WhitelistCandidateRequest{
					CandidateAddress: addr.MarshalPB(),
					Amount:           &types.BigUInt{Value: loom.BigUInt{big.NewInt(1000000000000)}},
					LockTime:         10,
				}},
				Meta: &dtypes.BatchRequestMeta{
					BlockNumber: 1,
					TxIndex:     0,
					LogIndex:    0,
				},
			},
		},
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(pctx.WithSender(addr)), &RegisterCandidateRequest{
		PubKey: pubKey,
		Fee:    oldFee,
	})
	require.Nil(t, err)

	listResponse, err := dposContract.ListCandidates(contractpb.WrapPluginContext(pctx.WithSender(addr)), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, oldFee, listResponse.Candidates[0].Fee)
	assert.Equal(t, oldFee, listResponse.Candidates[0].NewFee)

	err = Elect(contractpb.WrapPluginContext(pctx.WithSender(addr)))
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(pctx.WithSender(addr)))
	require.Nil(t, err)

	// Fee should not reset
	listResponse, err = dposContract.ListCandidates(contractpb.WrapPluginContext(pctx.WithSender(addr)), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, oldFee, listResponse.Candidates[0].Fee)
	assert.Equal(t, oldFee, listResponse.Candidates[0].NewFee)

	err = dposContract.ChangeFee(contractpb.WrapPluginContext(pctx.WithSender(addr)), &dtypes.ChangeCandidateFeeRequest{
		Fee: newFee,
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(pctx.WithSender(addr)))
	require.Nil(t, err)

	listResponse, err = dposContract.ListCandidates(contractpb.WrapPluginContext(pctx.WithSender(addr)), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, oldFee, listResponse.Candidates[0].Fee)
	assert.Equal(t, newFee, listResponse.Candidates[0].NewFee)

	err = Elect(contractpb.WrapPluginContext(pctx.WithSender(addr)))
	require.Nil(t, err)

	listResponse, err = dposContract.ListCandidates(contractpb.WrapPluginContext(pctx.WithSender(addr)), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, newFee, listResponse.Candidates[0].Fee)
	assert.Equal(t, newFee, listResponse.Candidates[0].NewFee)
}

func TestDelegate(t *testing.T) {
	pubKey1, _ := hex.DecodeString(validatorPubKeyHex1)
	addr1 := loom.Address{
		Local: loom.LocalAddressFromPublicKey(pubKey1),
	}
	oraclePubKey, _ := hex.DecodeString(validatorPubKeyHex2)
	oracleAddr := loom.Address{
		Local: loom.LocalAddressFromPublicKey(oraclePubKey),
	}

	pctx := plugin.CreateFakeContext(addr1, addr1)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 1000000000000000000),
			makeAccount(delegatorAddress2, 2000000000000000000),
			makeAccount(delegatorAddress3, 1000000000000000000),
			makeAccount(addr1, 1000000000000000000),
		},
	})

	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(oracleAddr)), &InitRequest{
		Params: &Params{
			ValidatorCount: 21,
			OracleAddress:  oracleAddr.MarshalPB(),
		},
	})
	require.NoError(t, err)

	err = dposContract.ProcessRequestBatch(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RequestBatch{
		Batch: []*dtypes.BatchRequest{
			&dtypes.BatchRequest{
				Payload: &dtypes.BatchRequest_WhitelistCandidate{&WhitelistCandidateRequest{
					CandidateAddress: addr1.MarshalPB(),
					Amount:           &types.BigUInt{Value: loom.BigUInt{big.NewInt(1000000000000)}},
					LockTime:         10,
				}},
				Meta: &dtypes.BatchRequestMeta{
					BlockNumber: 1,
					TxIndex:     0,
					LogIndex:    0,
				},
			},
		},
	})
	require.Error(t, err)

	err = dposContract.ProcessRequestBatch(contractpb.WrapPluginContext(dposCtx.WithSender(oracleAddr)), &RequestBatch{
		Batch: []*dtypes.BatchRequest{
			&dtypes.BatchRequest{
				Payload: &dtypes.BatchRequest_WhitelistCandidate{&WhitelistCandidateRequest{
					CandidateAddress: addr1.MarshalPB(),
					Amount:           &types.BigUInt{Value: loom.BigUInt{big.NewInt(1000000000000)}},
					LockTime:         10,
				}},
				Meta: &dtypes.BatchRequestMeta{
					BlockNumber: 1,
					TxIndex:     0,
					LogIndex:    0,
				},
			},
		},
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RegisterCandidateRequest{
		PubKey: pubKey1,
	})
	require.Nil(t, err)

	delegationAmount := &types.BigUInt{Value: loom.BigUInt{big.NewInt(100)}}
	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  delegationAmount,
	})
	require.Nil(t, err)

	response, err := coinContract.Allowance(contractpb.WrapPluginContext(coinCtx.WithSender(oracleAddr)), &coin.AllowanceRequest{
		Owner:   addr1.MarshalPB(),
		Spender: dposAddr.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, delegationAmount.Value.Int64(), response.Amount.Value.Int64())

	listResponse, err := dposContract.ListCandidates(contractpb.WrapPluginContext(dposCtx.WithSender(oracleAddr)), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listResponse.Candidates), 1)
	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           delegationAmount,
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  delegationAmount,
	})
	require.Nil(t, err)

	// total rewards distribution should equal 0 before elections run
	rewardsResponse, err := dposContract.CheckRewards(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckRewardsRequest{})
	require.Nil(t, err)
	assert.True(t, rewardsResponse.TotalRewardDistribution.Value.Cmp(common.BigZero()) == 0)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// total rewards distribution should equal still be zero after first election
	rewardsResponse, err = dposContract.CheckRewards(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckRewardsRequest{})
	require.Nil(t, err)
	assert.True(t, rewardsResponse.TotalRewardDistribution.Value.Cmp(common.BigZero()) == 0)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           delegationAmount,
	})
	require.Nil(t, err)

	delegationResponse, err := dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
		DelegatorAddress: addr1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.True(t, delegationResponse.Amount.Value.Cmp(&delegationAmount.Value) == 0)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  delegationAmount,
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           delegationAmount,
	})
	require.Nil(t, err)

	// checking a non-existent delegation should result in an empty (amount = 0)
	// delegaiton being returned
	delegationResponse, err = dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
		DelegatorAddress: addr2.MarshalPB(),
	})
	require.Nil(t, err)
	assert.True(t, delegationResponse.Amount.Value.Cmp(common.BigZero()) == 0)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  delegationAmount,
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// total rewards distribution should be greater than zero
	rewardsResponse, err = dposContract.CheckRewards(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckRewardsRequest{})
	require.Nil(t, err)
	assert.True(t, rewardsResponse.TotalRewardDistribution.Value.Cmp(common.BigZero()) > 0)

	// advancing contract time beyond the delegator1-addr1 lock period
	now := uint64(dposCtx.Now().Unix())
	dposCtx.SetTime(dposCtx.Now().Add(time.Duration(now+TierLocktimeMap[0]) * time.Second))

	err = dposContract.Unbond(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &UnbondRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           delegationAmount,
		Index:            1,
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	err = dposContract.Unbond(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &UnbondRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           delegationAmount,
		Index:            2,
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	err = dposContract.Unbond(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &UnbondRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: loom.BigUInt{big.NewInt(1)}},
		Index:            3,
	})
	assert.True(t, err != nil)

	// testing delegations to limbo validator
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &RedelegateRequest{
		FormerValidatorAddress: addr1.MarshalPB(),
		ValidatorAddress:       limboValidatorAddress.MarshalPB(),
		Amount:                 delegationAmount,
		Index:                  1,
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	delegationResponse, err = dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
		DelegatorAddress: delegatorAddress1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.True(t, delegationResponse.Amount.Value.Cmp(common.BigZero()) == 0)

	delegationResponse, err = dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: limboValidatorAddress.MarshalPB(),
		DelegatorAddress: delegatorAddress1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.True(t, delegationResponse.Amount.Value.Cmp(&delegationAmount.Value) == 0)
}

func TestRedelegate(t *testing.T) {
	pubKey1, _ := hex.DecodeString(validatorPubKeyHex1)
	addr1 := loom.Address{
		Local: loom.LocalAddressFromPublicKey(pubKey1),
	}
	pubKey2, _ := hex.DecodeString(validatorPubKeyHex2)
	addr2 := loom.Address{
		Local: loom.LocalAddressFromPublicKey(pubKey2),
	}
	pubKey3, _ := hex.DecodeString(validatorPubKeyHex3)
	addr3 := loom.Address{
		Local: loom.LocalAddressFromPublicKey(pubKey3),
	}

	pctx := plugin.CreateFakeContext(addr1, addr1)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 1000000000000000000),
			makeAccount(delegatorAddress2, 2000000000000000000),
			makeAccount(delegatorAddress3, 1000000000000000000),
			makeAccount(addr1, 1000000000000000000),
			makeAccount(addr2, 1000000000000000000),
			makeAccount(addr3, 1000000000000000000),
		},
	})

	registrationFee := loom.BigZeroPB()

	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &InitRequest{
		Params: &Params{
			ValidatorCount:          21,
			RegistrationRequirement: registrationFee,
		},
	})
	require.NoError(t, err)

	// Registering 3 candidates
	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RegisterCandidateRequest{
		PubKey: pubKey1,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr2)), &RegisterCandidateRequest{
		PubKey: pubKey2,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr3)), &RegisterCandidateRequest{
		PubKey: pubKey3,
	})
	require.Nil(t, err)

	listResponse, err := dposContract.ListCandidates(contractpb.WrapPluginContext(dposCtx), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listResponse.Candidates), 3)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// Verifying that with registration fee = 0, none of the 3 registered candidates are elected validators
	listValidatorsResponse, err := dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 0)

	delegationAmount := loom.NewBigUIntFromInt(10000000)
	smallDelegationAmount := loom.NewBigUIntFromInt(1000000)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// Verifying that addr1 was elected sole validator
	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 1)
	assert.True(t, listValidatorsResponse.Statistics[0].Address.Local.Compare(addr1.Local) == 0)

	// checking that redelegation fails with 0 amount
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &RedelegateRequest{
		FormerValidatorAddress: addr1.MarshalPB(),
		ValidatorAddress:       addr2.MarshalPB(),
		Amount:                 loom.BigZeroPB(),
		Index:                  1,
	})
	require.NotNil(t, err)

	// redelegating sole delegation to validator addr2
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &RedelegateRequest{
		FormerValidatorAddress: addr1.MarshalPB(),
		ValidatorAddress:       addr2.MarshalPB(),
		Amount:                 &types.BigUInt{Value: *delegationAmount},
		Index:                  1,
	})
	require.Nil(t, err)

	// Redelegation takes effect within a single election period
	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// Verifying that addr2 was elected sole validator
	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 1)
	assert.True(t, listValidatorsResponse.Statistics[0].Address.Local.Compare(addr2.Local) == 0)

	// redelegating sole delegation to validator addr3
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &RedelegateRequest{
		FormerValidatorAddress: addr2.MarshalPB(),
		ValidatorAddress:       addr3.MarshalPB(),
		Amount:                 &types.BigUInt{Value: *delegationAmount},
		Index:                  1,
	})
	require.Nil(t, err)

	// Redelegation takes effect within a single election period
	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// Verifying that addr3 was elected sole validator
	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 1)
	assert.True(t, listValidatorsResponse.Statistics[0].Address.Local.Compare(addr3.Local) == 0)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress2)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	// adding 2nd delegation from 2nd delegator in order to elect a second validator
	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// checking that the 2nd validator (addr1) was elected in addition to add3
	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 2)

	// delegator 1 removes delegation to limbo
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &RedelegateRequest{
		FormerValidatorAddress: addr3.MarshalPB(),
		ValidatorAddress:       limboValidatorAddress.MarshalPB(),
		Amount:                 &types.BigUInt{Value: *delegationAmount},
		Index:                  1,
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// Verifying that addr1 was elected sole validator AFTER delegator1 redelegated to limbo validator
	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 1)
	assert.True(t, listValidatorsResponse.Statistics[0].Address.Local.Compare(addr1.Local) == 0)

	// Checking that redelegaiton of a negative amount is rejected
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &RedelegateRequest{
		FormerValidatorAddress: addr1.MarshalPB(),
		ValidatorAddress:       addr2.MarshalPB(),
		Amount:                 &types.BigUInt{Value: *loom.NewBigUIntFromInt(-1000)},
	})
	require.NotNil(t, err)

	// Checking that redelegaiton of an amount greater than the total delegation is rejected
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &RedelegateRequest{
		FormerValidatorAddress: addr1.MarshalPB(),
		ValidatorAddress:       addr2.MarshalPB(),
		Amount:                 &types.BigUInt{Value: *loom.NewBigUIntFromInt(100000000)},
	})
	require.NotNil(t, err)

	// splitting delegator2's delegation to 2nd validator
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &RedelegateRequest{
		FormerValidatorAddress: addr1.MarshalPB(),
		ValidatorAddress:       addr2.MarshalPB(),
		Amount:                 &types.BigUInt{Value: *smallDelegationAmount},
		Index:                  1,
	})
	require.Nil(t, err)

	// splitting delegator2's delegation to 3rd validator
	// this also tests that redelegate is able to set a new tier
	err = dposContract.Redelegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &RedelegateRequest{
		FormerValidatorAddress: addr1.MarshalPB(),
		ValidatorAddress:       addr3.MarshalPB(),
		Amount:                 &types.BigUInt{Value: *smallDelegationAmount},
		NewLocktimeTier:        3,
		Index:                  1,
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	delegationResponse, err := dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: addr3.MarshalPB(),
		DelegatorAddress: delegatorAddress2.MarshalPB(),
	})
	require.Nil(t, err)
	assert.True(t, delegationResponse.Amount.Value.Cmp(smallDelegationAmount) == 0)
	assert.Equal(t, delegationResponse.Delegations[len(delegationResponse.Delegations)-1].LocktimeTier, TIER_THREE)

	// checking that all 3 candidates have been elected validators
	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 3)
}

func TestReward(t *testing.T) {
	// set elect time in params to one second for easy calculations
	delegationAmount := loom.BigUInt{big.NewInt(10000000000000)}
	cycleLengthSeconds := int64(100)
	params := Params{
		ElectionCycleLength: cycleLengthSeconds,
		MaxYearlyReward:     &types.BigUInt{Value: *scientificNotation(defaultMaxYearlyReward, tokenDecimals)},
	}
	statistic := ValidatorStatistic{
		DistributionTotal: &types.BigUInt{Value: loom.BigUInt{big.NewInt(0)}},
		DelegationTotal:   &types.BigUInt{Value: delegationAmount},
	}
	for i := int64(0); i < yearSeconds; i = i + cycleLengthSeconds {
		rewardValidator(&statistic, &params, *common.BigZero())
	}

	// checking that distribution is roughtly equal to 5% of delegation after one year
	assert.Equal(t, statistic.DistributionTotal.Value.Cmp(&loom.BigUInt{big.NewInt(490000000000)}), 1)
	assert.Equal(t, statistic.DistributionTotal.Value.Cmp(&loom.BigUInt{big.NewInt(510000000000)}), -1)
}

func TestElect(t *testing.T) {
	chainID := "chain"
	pubKey1, _ := hex.DecodeString(validatorPubKeyHex1)
	addr1 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey1),
	}

	pubKey2, _ := hex.DecodeString(validatorPubKeyHex2)
	addr2 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey2),
	}

	pubKey3, _ := hex.DecodeString(validatorPubKeyHex3)
	addr3 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey3),
	}

	// Init the coin balances
	var startTime int64 = 100000
	pctx := plugin.CreateFakeContext(delegatorAddress1, loom.Address{}).WithBlock(loom.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 130),
			makeAccount(delegatorAddress2, 20),
			makeAccount(delegatorAddress3, 10),
		},
	})

	// create dpos contract
	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)

	// transfer coins to reward fund
	amount := big.NewInt(10)
	amount.Exp(amount, big.NewInt(19), nil)
	coinContract.Transfer(contractpb.WrapPluginContext(coinCtx), &coin.TransferRequest{
		To: dposAddr.MarshalPB(),
		Amount: &types.BigUInt{
			Value: common.BigUInt{amount},
		},
	})

	// Init the dpos contract
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &InitRequest{
		Params: &Params{
			CoinContractAddress: coinAddr.MarshalPB(),
			ValidatorCount:      2,
			ElectionCycleLength: 0,
			OracleAddress:       addr1.MarshalPB(),
		},
	})
	require.Nil(t, err)

	err = dposContract.ProcessRequestBatch(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RequestBatch{
		Batch: []*dtypes.BatchRequest{
			&dtypes.BatchRequest{
				Payload: &dtypes.BatchRequest_WhitelistCandidate{&WhitelistCandidateRequest{
					CandidateAddress: addr1.MarshalPB(),
					Amount:           &types.BigUInt{Value: loom.BigUInt{big.NewInt(1000000000000)}},
					LockTime:         10,
				}},
				Meta: &dtypes.BatchRequestMeta{
					BlockNumber: 1,
					TxIndex:     0,
					LogIndex:    0,
				},
			},
		},
	})
	require.Nil(t, err)

	whitelistAmount := loom.BigUInt{big.NewInt(1000000000000)}

	err = dposContract.ProcessRequestBatch(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RequestBatch{
		Batch: []*dtypes.BatchRequest{
			&dtypes.BatchRequest{
				Payload: &dtypes.BatchRequest_WhitelistCandidate{&WhitelistCandidateRequest{
					CandidateAddress: addr2.MarshalPB(),
					Amount:           &types.BigUInt{Value: whitelistAmount},
					LockTime:         10,
				}},
				Meta: &dtypes.BatchRequestMeta{
					BlockNumber: 2,
					TxIndex:     0,
					LogIndex:    0,
				},
			},
		},
	})
	require.Nil(t, err)

	err = dposContract.ProcessRequestBatch(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RequestBatch{
		Batch: []*dtypes.BatchRequest{
			&dtypes.BatchRequest{
				Payload: &dtypes.BatchRequest_WhitelistCandidate{&WhitelistCandidateRequest{
					CandidateAddress: addr3.MarshalPB(),
					Amount:           &types.BigUInt{Value: whitelistAmount},
					LockTime:         10,
				}},
				Meta: &dtypes.BatchRequestMeta{
					BlockNumber: 3,
					TxIndex:     0,
					LogIndex:    0,
				},
			},
		},
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RegisterCandidateRequest{
		PubKey: pubKey1,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr2)), &RegisterCandidateRequest{
		PubKey: pubKey2,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr3)), &RegisterCandidateRequest{
		PubKey: pubKey3,
	})
	require.Nil(t, err)

	listCandidatesResponse, err := dposContract.ListCandidates(contractpb.WrapPluginContext(dposCtx), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listCandidatesResponse.Candidates), 3)

	listValidatorsResponse, err := dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 0)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 2)

	oldRewardsValue := *common.BigZero()
	for i := 0; i < 10; i++ {
		err = Elect(contractpb.WrapPluginContext(dposCtx))
		require.Nil(t, err)
		checkDelegation, _ := dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
			ValidatorAddress: addr1.MarshalPB(),
			DelegatorAddress: addr1.MarshalPB(),
		})
		// get rewards delegaiton which is always at index 0
		delegation := checkDelegation.Delegations[REWARD_DELEGATION_INDEX]
		assert.Equal(t, delegation.Amount.Value.Cmp(&oldRewardsValue), 1)
		oldRewardsValue = delegation.Amount.Value
	}

	// Change WhitelistAmount and verify that it got changed correctly
	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	validator := listValidatorsResponse.Statistics[0]
	assert.Equal(t, whitelistAmount, validator.WhitelistAmount.Value)

	newWhitelistAmount := loom.BigUInt{big.NewInt(2000000000000)}

	// only oracle
	err = dposContract.ChangeWhitelistAmount(contractpb.WrapPluginContext(dposCtx.WithSender(addr2)), &ChangeWhitelistAmountRequest{
		CandidateAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: newWhitelistAmount},
	})
	require.Error(t, err)

	err = dposContract.ChangeWhitelistAmount(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &ChangeWhitelistAmountRequest{
		CandidateAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: newWhitelistAmount},
	})
	require.Nil(t, err)

	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	validator = listValidatorsResponse.Statistics[0]
	assert.Equal(t, newWhitelistAmount, validator.WhitelistAmount.Value)
}

func TestValidatorRewards(t *testing.T) {
	chainID := "chain"
	pubKey1, _ := hex.DecodeString(validatorPubKeyHex1)
	addr1 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey1),
	}
	pubKey2, _ := hex.DecodeString(validatorPubKeyHex2)
	addr2 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey2),
	}
	pubKey3, _ := hex.DecodeString(validatorPubKeyHex3)
	addr3 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey3),
	}

	// Init the coin balances
	var startTime int64 = 100000
	pctx := plugin.CreateFakeContext(delegatorAddress1, loom.Address{}).WithBlock(loom.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 100000000),
			makeAccount(delegatorAddress2, 100000000),
			makeAccount(delegatorAddress3, 100000000),
			makeAccount(addr1, 100000000),
			makeAccount(addr2, 100000000),
			makeAccount(addr3, 100000000),
		},
	})

	// create dpos contract
	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)

	// transfer coins to reward fund
	amount := big.NewInt(10)
	amount.Exp(amount, big.NewInt(19), nil)
	coinContract.Transfer(contractpb.WrapPluginContext(coinCtx), &coin.TransferRequest{
		To: dposAddr.MarshalPB(),
		Amount: &types.BigUInt{
			Value: common.BigUInt{amount},
		},
	})

	// Init the dpos contract
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &InitRequest{
		Params: &Params{
			CoinContractAddress: coinAddr.MarshalPB(),
			ValidatorCount:      10,
			ElectionCycleLength: 0,
		},
	})
	require.Nil(t, err)

	registrationFee := &types.BigUInt{Value: *scientificNotation(defaultRegistrationRequirement, tokenDecimals)}

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RegisterCandidateRequest{
		PubKey: pubKey1,
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr2)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr2)), &RegisterCandidateRequest{
		PubKey: pubKey2,
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr3)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr3)), &RegisterCandidateRequest{
		PubKey: pubKey3,
	})
	require.Nil(t, err)

	listCandidatesResponse, err := dposContract.ListCandidates(contractpb.WrapPluginContext(dposCtx), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listCandidatesResponse.Candidates), 3)

	listValidatorsResponse, err := dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 0)
	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 3)

	// Two delegators delegate 1/2 and 1/4 of a registration fee respectively
	smallDelegationAmount := loom.NewBigUIntFromInt(0)
	smallDelegationAmount.Div(&registrationFee.Value, loom.NewBigUIntFromInt(4))
	largeDelegationAmount := loom.NewBigUIntFromInt(0)
	largeDelegationAmount.Div(&registrationFee.Value, loom.NewBigUIntFromInt(2))

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress2)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *largeDelegationAmount},
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *largeDelegationAmount},
	})
	require.Nil(t, err)

	for i := 0; i < 10000; i++ {
		err = Elect(contractpb.WrapPluginContext(dposCtx))
		require.Nil(t, err)
	}

	// TODO create table-based test of validator rewards here
}

func TestRewardTiers(t *testing.T) {
	chainID := "chain"
	pubKey1, _ := hex.DecodeString(validatorPubKeyHex1)
	addr1 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey1),
	}
	pubKey2, _ := hex.DecodeString(validatorPubKeyHex2)
	addr2 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey2),
	}
	pubKey3, _ := hex.DecodeString(validatorPubKeyHex3)
	addr3 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey3),
	}

	// Init the coin balances
	var startTime int64 = 100000
	pctx := plugin.CreateFakeContext(delegatorAddress1, loom.Address{}).WithBlock(loom.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 100000000),
			makeAccount(delegatorAddress2, 100000000),
			makeAccount(delegatorAddress3, 100000000),
			makeAccount(delegatorAddress4, 100000000),
			makeAccount(delegatorAddress5, 100000000),
			makeAccount(addr1, 100000000),
			makeAccount(addr2, 100000000),
			makeAccount(addr3, 100000000),
		},
	})

	// create dpos contract
	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)

	// transfer coins to reward fund
	amount := big.NewInt(10)
	amount.Exp(amount, big.NewInt(19), nil)
	coinContract.Transfer(contractpb.WrapPluginContext(coinCtx), &coin.TransferRequest{
		To: dposAddr.MarshalPB(),
		Amount: &types.BigUInt{
			Value: common.BigUInt{amount},
		},
	})

	// Init the dpos contract
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &InitRequest{
		Params: &Params{
			CoinContractAddress: coinAddr.MarshalPB(),
			ValidatorCount:      10,
			ElectionCycleLength: 0,
		},
	})
	require.Nil(t, err)

	registrationFee := &types.BigUInt{Value: *scientificNotation(defaultRegistrationRequirement, tokenDecimals)}

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RegisterCandidateRequest{
		PubKey: pubKey1,
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr2)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr2)), &RegisterCandidateRequest{
		PubKey: pubKey2,
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr3)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr3)), &RegisterCandidateRequest{
		PubKey: pubKey3,
	})
	require.Nil(t, err)

	listCandidatesResponse, err := dposContract.ListCandidates(contractpb.WrapPluginContext(dposCtx), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listCandidatesResponse.Candidates), 3)

	listValidatorsResponse, err := dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 0)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 3)

	smallDelegationAmount := loom.NewBigUIntFromInt(0)
	smallDelegationAmount.Div(&registrationFee.Value, loom.NewBigUIntFromInt(4))
	largeDelegationAmount := loom.NewBigUIntFromInt(0)
	largeDelegationAmount.Div(&registrationFee.Value, loom.NewBigUIntFromInt(2))

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	// LocktimeTier should default to 0 for delegatorAddress1
	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress2)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *smallDelegationAmount},
		LocktimeTier:     2,
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress3)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress3)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *smallDelegationAmount},
		LocktimeTier:     3,
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress4)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress4)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *smallDelegationAmount},
		LocktimeTier:     1,
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress5)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *largeDelegationAmount},
	})
	require.Nil(t, err)

	// Though Delegator5 delegates to addr2 and not addr1 like the rest of the
	// delegators, he should still receive the same rewards proportional to his
	// delegation parameters
	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress5)), &DelegateRequest{
		ValidatorAddress: addr2.MarshalPB(),
		Amount:           &types.BigUInt{Value: *largeDelegationAmount},
		LocktimeTier:     2,
	})
	require.Nil(t, err)

	for i := 0; i < 10000; i++ {
		err = Elect(contractpb.WrapPluginContext(dposCtx))
		require.Nil(t, err)
	}

	addr1Claim, err := dposContract.CheckRewardDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckRewardDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, addr1Claim.Delegation.Amount.Value.Cmp(common.BigZero()), 1)

	delegator1Claim, err := dposContract.CheckRewardDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &CheckRewardDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, delegator1Claim.Delegation.Amount.Value.Cmp(common.BigZero()), 1)

	delegator2Claim, err := dposContract.CheckRewardDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &CheckRewardDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, delegator2Claim.Delegation.Amount.Value.Cmp(common.BigZero()), 1)

	delegator3Claim, err := dposContract.CheckRewardDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress3)), &CheckRewardDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, delegator3Claim.Delegation.Amount.Value.Cmp(common.BigZero()), 1)

	delegator4Claim, err := dposContract.CheckRewardDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress4)), &CheckRewardDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, delegator4Claim.Delegation.Amount.Value.Cmp(common.BigZero()), 1)

	delegator5Claim, err := dposContract.CheckRewardDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress5)), &CheckRewardDelegationRequest{
		ValidatorAddress: addr2.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, delegator5Claim.Delegation.Amount.Value.Cmp(common.BigZero()), 1)

	maximumDifference := scientificNotation(1, tokenDecimals)
	difference := loom.NewBigUIntFromInt(0)

	// Checking that Delegator2's claim is almost exactly twice Delegator1's claim
	scaledDelegator1Claim := CalculateFraction(*loom.NewBigUIntFromInt(20000), delegator1Claim.Delegation.Amount.Value)
	difference.Sub(&scaledDelegator1Claim, &delegator2Claim.Delegation.Amount.Value)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)

	// Checking that Delegator3's & Delegator5's claim is almost exactly four times Delegator1's claim
	scaledDelegator1Claim = CalculateFraction(*loom.NewBigUIntFromInt(40000), delegator1Claim.Delegation.Amount.Value)

	difference.Sub(&scaledDelegator1Claim, &delegator3Claim.Delegation.Amount.Value)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)

	difference.Sub(&scaledDelegator1Claim, &delegator5Claim.Delegation.Amount.Value)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)

	// Checking that Delegator4's claim is almost exactly 1.5 times Delegator1's claim
	scaledDelegator1Claim = CalculateFraction(*loom.NewBigUIntFromInt(15000), delegator1Claim.Delegation.Amount.Value)
	difference.Sub(&scaledDelegator1Claim, &delegator4Claim.Delegation.Amount.Value)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)

	// Testing total delegation functionality

	checkAllDelegationsResponse, err := dposContract.CheckAllDelegations(contractpb.WrapPluginContext(dposCtx), &CheckAllDelegationsRequest{
		DelegatorAddress: delegatorAddress3.MarshalPB(),
	})
	require.Nil(t, err)
	assert.True(t, checkAllDelegationsResponse.Amount.Value.Cmp(smallDelegationAmount) > 0)
	expectedWeightedAmount := CalculateFraction(*loom.NewBigUIntFromInt(40000), *smallDelegationAmount)
	assert.True(t, checkAllDelegationsResponse.WeightedAmount.Value.Cmp(&expectedWeightedAmount) > 0)
}

// Besides reward cap functionality, this also demostrates 0-fee candidate registration
func TestRewardCap(t *testing.T) {
	chainID := "chain"
	pubKey1, _ := hex.DecodeString(validatorPubKeyHex1)
	addr1 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey1),
	}
	pubKey2, _ := hex.DecodeString(validatorPubKeyHex2)
	addr2 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey2),
	}
	pubKey3, _ := hex.DecodeString(validatorPubKeyHex3)
	addr3 := loom.Address{
		ChainID: chainID,
		Local:   loom.LocalAddressFromPublicKey(pubKey3),
	}

	// Init the coin balances
	var startTime int64 = 100000
	pctx := plugin.CreateFakeContext(delegatorAddress1, loom.Address{}).WithBlock(loom.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 100000000),
			makeAccount(delegatorAddress2, 100000000),
			makeAccount(delegatorAddress3, 100000000),
			makeAccount(addr1, 100000000),
			makeAccount(addr2, 100000000),
			makeAccount(addr3, 100000000),
		},
	})

	// create dpos contract
	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)

	// transfer coins to reward fund
	amount := big.NewInt(10)
	amount.Exp(amount, big.NewInt(19), nil)
	coinContract.Transfer(contractpb.WrapPluginContext(coinCtx), &coin.TransferRequest{
		To: dposAddr.MarshalPB(),
		Amount: &types.BigUInt{
			Value: common.BigUInt{amount},
		},
	})

	registrationFee := loom.BigZeroPB()

	// Init the dpos contract
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &InitRequest{
		Params: &Params{
			CoinContractAddress: coinAddr.MarshalPB(),
			ValidatorCount:      10,
			ElectionCycleLength: 0,
			MaxYearlyReward:     &types.BigUInt{Value: *scientificNotation(100, tokenDecimals)},
			// setting registration fee to zero for easy calculations using delegations alone
			RegistrationRequirement: registrationFee,
		},
	})

	require.Nil(t, err)
	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RegisterCandidateRequest{
		PubKey: pubKey1,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr2)), &RegisterCandidateRequest{
		PubKey: pubKey2,
	})
	require.Nil(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr3)), &RegisterCandidateRequest{
		PubKey: pubKey3,
	})
	require.Nil(t, err)

	listCandidatesResponse, err := dposContract.ListCandidates(contractpb.WrapPluginContext(dposCtx), &ListCandidateRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listCandidatesResponse.Candidates), 3)

	listValidatorsResponse, err := dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 0)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 0)

	delegationAmount := scientificNotation(1000, tokenDecimals)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &DelegateRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress2)), &coin.ApproveRequest{
		Spender: dposAddr.MarshalPB(),
		Amount:  &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &DelegateRequest{
		ValidatorAddress: addr2.MarshalPB(),
		Amount:           &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	// With a default yearly reward of 5% of one's token holdings, the two
	// delegators should reach their rewards limits by both delegating exactly
	// 1000, or 2000 combined since 2000 = 100 (the max yearly reward) / 0.05

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	listValidatorsResponse, err = dposContract.ListValidators(contractpb.WrapPluginContext(dposCtx), &ListValidatorsRequest{})
	require.Nil(t, err)
	assert.Equal(t, len(listValidatorsResponse.Statistics), 2)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	// delegator1Claim, err := dposContract.ClaimDistribution(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &ClaimDistributionRequest{
	// 	WithdrawalAddress: delegatorAddress1.MarshalPB(),
	// })
	// require.Nil(t, err)
	// assert.Equal(t, delegator1Claim.Amount.Value.Cmp(&loom.BigUInt{big.NewInt(0)}), 1)

	// delegator2Claim, err := dposContract.ClaimDistribution(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress2)), &ClaimDistributionRequest{
	// 	WithdrawalAddress: delegatorAddress2.MarshalPB(),
	// })
	// require.Nil(t, err)
	// assert.Equal(t, delegator2Claim.Amount.Value.Cmp(&loom.BigUInt{big.NewInt(0)}), 1)

	// //                           |---- this 2 is the election cycle length used when,
	// //    v--- delegationAmount  v     for testing, a 0-sec election time is set
	// // ((1000 * 10**18) * 0.05 * 2) / (365 * 24 * 3600) = 3.1709791983764585e12
	// expectedAmount := loom.NewBigUIntFromInt(3170979198376)
	// assert.Equal(t, *expectedAmount, delegator2Claim.Amount.Value)

	// err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress3)), &coin.ApproveRequest{
	// 	Spender: dposAddr.MarshalPB(),
	// 	Amount:  &types.BigUInt{Value: *delegationAmount},
	// })
	// require.Nil(t, err)

	// err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress3)), &DelegateRequest{
	// 	ValidatorAddress: addr1.MarshalPB(),
	// 	Amount:           &types.BigUInt{Value: *delegationAmount},
	// })
	// require.Nil(t, err)

	// // run one election to get Delegator3 elected as a validator
	// err = Elect(contractpb.WrapPluginContext(dposCtx))
	// require.Nil(t, err)

	// // run another election to get Delegator3 his first reward distribution
	// err = Elect(contractpb.WrapPluginContext(dposCtx))
	// require.Nil(t, err)

	// delegator3Claim, err := dposContract.ClaimDistribution(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress3)), &ClaimDistributionRequest{
	// 	WithdrawalAddress: delegatorAddress3.MarshalPB(),
	// })
	// require.Nil(t, err)
	// assert.Equal(t, delegator3Claim.Amount.Value.Cmp(&loom.BigUInt{big.NewInt(0)}), 1)

	// // verifiying that claim is smaller than what was given when delegations
	// // were smaller and below max yearly reward cap.
	// // delegator3Claim should be ~2/3 of delegator2Claim
	// assert.Equal(t, delegator2Claim.Amount.Value.Cmp(&delegator3Claim.Amount.Value), 1)
	// scaledDelegator3Claim := CalculateFraction(*loom.NewBigUIntFromInt(15000), delegator3Claim.Amount.Value)
	// difference := common.BigZero()
	// difference.Sub(&scaledDelegator3Claim, &delegator2Claim.Amount.Value)
	// // amounts must be within 3 * 10^-18 tokens of each other to be correct
	// maximumDifference := loom.NewBigUIntFromInt(3)
	// assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)
}

func TestMultiDelegate(t *testing.T) {
	pubKey1, _ := hex.DecodeString(validatorPubKeyHex1)
	addr1 := loom.Address{
		Local: loom.LocalAddressFromPublicKey(pubKey1),
	}

	pctx := plugin.CreateFakeContext(addr1, addr1)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 1000000000000000000),
			makeAccount(addr1, 1000000000000000000),
		},
	})

	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &InitRequest{
		Params: &Params{
			ValidatorCount: 21,
			RegistrationRequirement: loom.BigZeroPB(),
		},
	})
	require.NoError(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RegisterCandidateRequest{
		PubKey: pubKey1,
	})
	require.Nil(t, err)

	delegationAmount := &types.BigUInt{Value: loom.BigUInt{big.NewInt(2000)}}
	numberOfDelegations := int64(200)

	for i := uint64(0); i < uint64(numberOfDelegations); i++ {
		err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
			Spender: dposAddr.MarshalPB(),
			Amount:  delegationAmount,
		})
		require.Nil(t, err)

		err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &DelegateRequest{
			ValidatorAddress: addr1.MarshalPB(),
			Amount:           delegationAmount,
			LocktimeTier:     i % 4, // testing delegations with a variety of locktime tiers
		})
		require.Nil(t, err)

		err = Elect(contractpb.WrapPluginContext(dposCtx))
		require.Nil(t, err)
	}

	delegationResponse, err := dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
		DelegatorAddress: addr1.MarshalPB(),
	})
	require.Nil(t, err)
	expectedAmount := common.BigZero()
	expectedAmount = expectedAmount.Mul(&delegationAmount.Value, &loom.BigUInt{big.NewInt(numberOfDelegations)})
	assert.True(t, delegationResponse.Amount.Value.Cmp(expectedAmount) == 0)
	// we add one to account for the rewards delegation
	assert.True(t, len(delegationResponse.Delegations) == int(numberOfDelegations + 1))

	numDelegations := DelegationsCount(contractpb.WrapPluginContext(dposCtx))
	assert.Equal(t, numDelegations, 201)

	for i := uint64(0); i < uint64(numberOfDelegations); i++ {
		err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
			Spender: dposAddr.MarshalPB(),
			Amount:  delegationAmount,
		})
		require.Nil(t, err)

		err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(delegatorAddress1)), &DelegateRequest{
			ValidatorAddress: addr1.MarshalPB(),
			Amount:           delegationAmount,
			LocktimeTier:     i % 4, // testing delegations with a variety of locktime tiers
		})
		require.Nil(t, err)

		err = Elect(contractpb.WrapPluginContext(dposCtx))
		require.Nil(t, err)
	}

	delegationResponse, err = dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
		DelegatorAddress: delegatorAddress1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.True(t, delegationResponse.Amount.Value.Cmp(expectedAmount) == 0)
	assert.True(t, len(delegationResponse.Delegations) == int(numberOfDelegations + 1))

	numDelegations = DelegationsCount(contractpb.WrapPluginContext(dposCtx))
	assert.Equal(t, numDelegations, 402)

	// advance contract time enough to unlock all delegations
	now := uint64(dposCtx.Now().Unix())
	dposCtx.SetTime(dposCtx.Now().Add(time.Duration(now+TierLocktimeMap[3]+1) * time.Second))

	err = dposContract.Unbond(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &UnbondRequest{
		ValidatorAddress: addr1.MarshalPB(),
		Amount:           delegationAmount,
		Index:            100,
	})
	require.Nil(t, err)

	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	numDelegations = DelegationsCount(contractpb.WrapPluginContext(dposCtx))
	assert.Equal(t, numDelegations, 402 - 1)

	// Check that all delegations have had thier tier reset to TIER_ZERO
	listAllDelegationsResponse, err := dposContract.ListAllDelegations(contractpb.WrapPluginContext(dposCtx), &ListAllDelegationsRequest{})
	require.Nil(t, err)

	for _, listDelegationsResponse := range listAllDelegationsResponse.ListResponses {
		for _, delegation := range listDelegationsResponse.Delegations {
			assert.Equal(t, delegation.LocktimeTier, TIER_ZERO)
		}
	}
}

func TestLockup(t *testing.T) {
	pubKey1, _ := hex.DecodeString(validatorPubKeyHex1)
	addr1 := loom.Address{
		Local: loom.LocalAddressFromPublicKey(pubKey1),
	}

	pctx := plugin.CreateFakeContext(addr1, addr1)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(addr1, 1000000000000000000),
			makeAccount(delegatorAddress1, 1000000000000000000),
			makeAccount(delegatorAddress2, 1000000000000000000),
			makeAccount(delegatorAddress3, 1000000000000000000),
			makeAccount(delegatorAddress4, 1000000000000000000),
		},
	})

	dposContract := &DPOS{}
	dposAddr := pctx.CreateContract(contractpb.MakePluginContract(dposContract))
	dposCtx := pctx.WithAddress(dposAddr)
	err := dposContract.Init(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &InitRequest{
		Params: &Params{
			ValidatorCount: 21,
			RegistrationRequirement: loom.BigZeroPB(),
		},
	})
	require.NoError(t, err)

	err = dposContract.RegisterCandidate(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &RegisterCandidateRequest{
		PubKey: pubKey1,
	})
	require.Nil(t, err)

	now := uint64(dposCtx.Now().Unix())
	delegationAmount := &types.BigUInt{Value: loom.BigUInt{big.NewInt(2000)}}

	var tests = []struct {
		Delegator loom.Address
		Tier      uint64
	}{
		{delegatorAddress1, 0},
		{delegatorAddress2, 1},
		{delegatorAddress3, 2},
		{delegatorAddress4, 3},
	}

	for _, test := range tests {
		expectedLockup := now + TierLocktimeMap[LocktimeTier(test.Tier)]

		// delegating
		err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(test.Delegator)), &coin.ApproveRequest{
			Spender: dposAddr.MarshalPB(),
			Amount:  delegationAmount,
		})
		require.Nil(t, err)

		err = dposContract.Delegate(contractpb.WrapPluginContext(dposCtx.WithSender(test.Delegator)), &DelegateRequest{
			ValidatorAddress: addr1.MarshalPB(),
			Amount:           delegationAmount,
			LocktimeTier:     test.Tier,
		})
		require.Nil(t, err)

		// checking delegation pre-election
		checkDelegation, err := dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
			ValidatorAddress: addr1.MarshalPB(),
			DelegatorAddress: test.Delegator.MarshalPB(),
		})
		delegation := checkDelegation.Delegations[len(checkDelegation.Delegations)-1]

		assert.Equal(t, expectedLockup, delegation.LockTime)
		assert.Equal(t, true, uint64(delegation.LocktimeTier) == test.Tier)
		assert.Equal(t, delegation.Amount.Value.Cmp(common.BigZero()), 0)
		assert.Equal(t, delegation.UpdateAmount.Value.Cmp(&delegationAmount.Value), 0)

		// running election
		err = Elect(contractpb.WrapPluginContext(dposCtx))
		require.Nil(t, err)

		// chekcing delegation post-election
		checkDelegation, err = dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
			ValidatorAddress: addr1.MarshalPB(),
			DelegatorAddress: test.Delegator.MarshalPB(),
		})
		delegation = checkDelegation.Delegations[len(checkDelegation.Delegations)-1]

		assert.Equal(t, expectedLockup, delegation.LockTime)
		assert.Equal(t, true, uint64(delegation.LocktimeTier) == test.Tier)
		assert.Equal(t, delegation.UpdateAmount.Value.Cmp(common.BigZero()), 0)
		assert.Equal(t, delegation.Amount.Value.Cmp(&delegationAmount.Value), 0)
	}

	// setting time to reset tiers of all delegations except the last
	dposCtx.SetTime(dposCtx.Now().Add(time.Duration(now+TierLocktimeMap[2]+1) * time.Second))

	// running election to trigger locktime resets
	err = Elect(contractpb.WrapPluginContext(dposCtx))
	require.Nil(t, err)

	delegationResponse, err := dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
		DelegatorAddress: delegatorAddress3.MarshalPB(),
	})
	assert.Equal(t, TIER_ZERO, delegationResponse.Delegations[len(delegationResponse.Delegations)-1].LocktimeTier)

	delegationResponse, err = dposContract.CheckDelegation(contractpb.WrapPluginContext(dposCtx.WithSender(addr1)), &CheckDelegationRequest{
		ValidatorAddress: addr1.MarshalPB(),
		DelegatorAddress: delegatorAddress4.MarshalPB(),
	})
	assert.Equal(t, TIER_THREE, delegationResponse.Delegations[len(delegationResponse.Delegations)-1].LocktimeTier)
}

func TestApplyPowerCap(t *testing.T) {
	var tests = []struct {
		input  []*Validator
		output []*Validator
	}{
		{
			[]*Validator{&Validator{Power: 10}},
			[]*Validator{&Validator{Power: 10}},
		},
		{
			[]*Validator{&Validator{Power: 10}, &Validator{Power: 1}},
			[]*Validator{&Validator{Power: 10}, &Validator{Power: 1}},
		},
		{
			[]*Validator{&Validator{Power: 30}, &Validator{Power: 30}, &Validator{Power: 30}, &Validator{Power: 30}},
			[]*Validator{&Validator{Power: 30}, &Validator{Power: 30}, &Validator{Power: 30}, &Validator{Power: 30}},
		},
		{
			[]*Validator{&Validator{Power: 33}, &Validator{Power: 30}, &Validator{Power: 22}, &Validator{Power: 22}},
			[]*Validator{&Validator{Power: 29}, &Validator{Power: 29}, &Validator{Power: 24}, &Validator{Power: 24}},
		},
		{
			[]*Validator{&Validator{Power: 100}, &Validator{Power: 20}, &Validator{Power: 5}, &Validator{Power: 5}, &Validator{Power: 5}},
			[]*Validator{&Validator{Power: 37}, &Validator{Power: 35}, &Validator{Power: 20}, &Validator{Power: 20}, &Validator{Power: 20}},
		},
		{
			[]*Validator{&Validator{Power: 150}, &Validator{Power: 100}, &Validator{Power: 77}, &Validator{Power: 15}, &Validator{Power: 15}, &Validator{Power: 10}},
			[]*Validator{&Validator{Power: 102}, &Validator{Power: 102}, &Validator{Power: 86}, &Validator{Power: 24}, &Validator{Power: 24}, &Validator{Power: 19}},
		},

	}
	for _, test := range tests {
		output := applyPowerCap(test.input)
		for i, o := range output {
			assert.Equal(t, test.output[i].Power, o.Power)
		}
	}
}

// UTILITIES

func makeAccount(owner loom.Address, bal uint64) *coin.InitialAccount {
	return &coin.InitialAccount{
		Owner:   owner.MarshalPB(),
		Balance: bal,
	}
}
