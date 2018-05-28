// +build evm

package vm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/gogo/protobuf/proto"

	"crypto/sha256"
	"github.com/loomnetwork/go-loom"
	ltypes "github.com/loomnetwork/go-loom/types"
	"github.com/loomnetwork/loomchain"
	"github.com/loomnetwork/loomchain/store"
)

var rootKey = []byte("vmroot")

var LoomEvmFactory = func(state loomchain.State) VM {
	return *NewLoomEvm(state)
}

type LoomEvm struct {
	db  ethdb.Database
	evm Evm
}

func NewLoomEvm(loomState loomchain.State) *LoomEvm {
	p := new(LoomEvm)
	p.db = NewLoomEthdb(loomState)
	oldRoot, _ := p.db.Get(rootKey)
	_state, _ := state.New(common.BytesToHash(oldRoot), state.NewDatabase(p.db))
	p.evm = *NewEvmFrom(*_state)
	return p
}

func (levm LoomEvm) Create(caller loom.Address, code []byte) ([]byte, loom.Address, error) {
	return levm.evm.Create(caller, code)
}

func (levm LoomEvm) Call(caller, addr loom.Address, input []byte) ([]byte, error) {
	return levm.evm.Call(caller, addr, input)
}

func (levm LoomEvm) StaticCall(caller, addr loom.Address, input []byte) ([]byte, error) {
	return levm.evm.StaticCall(caller, addr, input)
}

func (levm LoomEvm) Commit() (common.Hash, error) {
	root, err := levm.evm.Commit()
	if err == nil {
		levm.db.Put(rootKey, root[:])
	}
	return root, err
}

var LoomVmFactory = func(state loomchain.State) VM {
	return NewLoomVm(state, nil)
}

type LoomVm struct {
	state        loomchain.State
	eventHandler loomchain.EventHandler
}

func NewLoomVm(loomState loomchain.State, eventHandler loomchain.EventHandler) VM {
	p := new(LoomVm)
	p.state = loomState
	p.eventHandler = eventHandler
	return p
}

func (lvm LoomVm) Create(caller loom.Address, code []byte) ([]byte, loom.Address, error) {
	levm := NewLoomEvm(lvm.state)
	ret, addr, err := levm.evm.Create(caller, code)
	if err == nil {
		_, err = levm.Commit()
	}
	lvm.postEvents(levm.evm.state.Logs(), caller, addr, code)
	return ret, addr, err
}

func (lvm LoomVm) Call(caller, addr loom.Address, input []byte) ([]byte, error) {
	levm := NewLoomEvm(lvm.state)
	_, err := levm.evm.Call(caller, addr, input)
	if err == nil {
		_, err = levm.Commit()
	}
	var events []*Event
	status := int32(0)
	if err == nil {
		events, _ = lvm.postEvents(levm.evm.state.Logs(), caller, addr, input)
		status = 1
	}
	storeState := *lvm.state.(*loomchain.StoreState)
	ssBlock := storeState.Block()
	ssBLastId := ssBlock.GetLastBlockID()
	txReceipt, err := proto.Marshal(&EvmTxReceipt{
		TransactionIndex:  ssBlock.NumTxs,
		BlockHash:         ssBLastId.Hash,
		BlockNumber:       ssBlock.Height,
		CumulativeGasUsed: 0,
		GasUsed:           0,
		ContractAddress:   addr.Local,
		Logs:              events,
		LogsBloom:         []byte{},
		Status:            status,
	})
	h := sha256.New()
	h.Write(txReceipt)
	txHash := h.Sum(nil)
	receiptState := store.PrefixKVStore(ReceiptPrefix, lvm.state)
	receiptState.Set(txHash, txReceipt)
	return txHash, err
}

func (lvm LoomVm) StaticCall(caller, addr loom.Address, input []byte) ([]byte, error) {
	levm := NewLoomEvm(lvm.state)
	ret, err := levm.evm.StaticCall(caller, addr, input)
	if err == nil {
		_, err = levm.Commit()
	}
	return ret, err
}

func (lvm LoomVm) postEvents(logs []*types.Log, caller, contract loom.Address, input []byte) ([]*Event, error) {
	var events []*Event
	if lvm.eventHandler == nil {
		return events, nil
	}
	for _, log := range logs {
		var topics [][]byte
		for _, topic := range log.Topics {
			topics = append(topics, topic.Bytes())
		}
		event := &Event{
			Contract: &ltypes.Address{
				ChainId: contract.ChainID,
				Local:   log.Address.Bytes(),
			},
			Topics: topics,
			Data:   log.Data,
		}
		events = append(events, event)
		flatLog, err := proto.Marshal(event)

		if err != nil {
			return []*Event{}, err
		}
		eventData := &loomchain.EventData{
			Caller:     caller,
			Address:    contract,
			PluginName: contract.String(),
			Data:       flatLog,
			RawRequest: input,
		}
		err = lvm.eventHandler.Post(lvm.state, eventData)
		if err != nil {
			return []*Event{}, err
		}
	}
	return events, nil
}
