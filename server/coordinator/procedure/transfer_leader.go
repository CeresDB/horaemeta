// Copyright 2022 CeresDB Project Authors. Licensed under Apache-2.0.

package procedure

import (
	"context"
	"sync"

	"github.com/CeresDB/ceresdbproto/pkg/clusterpb"
	"github.com/CeresDB/ceresmeta/server/cluster"
	"github.com/CeresDB/ceresmeta/server/coordinator/procedure/dispatch"
	"github.com/looplab/fsm"
	"github.com/pkg/errors"
)

const (
	EventTransferLeaderPrepare = "EventTransferLeaderPrepare"
	EventTransferLeaderFailed  = "EventTransferLeaderFailed"
	EventTransferLeaderSuccess = "EventTransferLeaderSuccess"

	StateTransferLeaderBegin   = "StateTransferLeaderBegin"
	StateTransferLeaderWaiting = "StateTransferLeaderWaiting"
	StateTransferLeaderFinish  = "StateTransferLeaderFinish"
	StateTransferLeaderFailed  = "StateTransferLeaderFailed"
)

var (
	transferLeaderEvents = fsm.Events{
		{Name: EventTransferLeaderPrepare, Src: []string{StateTransferLeaderBegin}, Dst: StateTransferLeaderWaiting},
		{Name: EventTransferLeaderSuccess, Src: []string{StateTransferLeaderWaiting}, Dst: StateTransferLeaderFinish},
		{Name: EventTransferLeaderFailed, Src: []string{StateTransferLeaderWaiting}, Dst: StateTransferLeaderFailed},
	}
	transferLeaderCallbacks = fsm.Callbacks{
		EventTransferLeaderPrepare: transferLeaderPrepareCallback,
		EventTransferLeaderFailed:  transferLeaderFailedCallback,
		EventTransferLeaderSuccess: transferLeaderSuccessCallback,
	}
)

type TransferLeaderProcedure struct {
	lock     sync.RWMutex
	fsm      *fsm.FSM
	id       uint64
	state    State
	dispatch dispatch.ActionDispatch
	cluster  *cluster.Cluster

	oldLeader *clusterpb.Shard
	newLeader *clusterpb.Shard
}

// TransferLeaderCallbackRequest is fsm callbacks param
type TransferLeaderCallbackRequest struct {
	cluster  *cluster.Cluster
	cxt      context.Context
	dispatch dispatch.ActionDispatch

	oldLeader *clusterpb.Shard
	newLeader *clusterpb.Shard
}

func NewTransferLeaderProcedure(dispatch dispatch.ActionDispatch, cluster *cluster.Cluster, oldLeader *clusterpb.Shard, newLeader *clusterpb.Shard, id uint64) Procedure {
	transferLeaderOperationFsm := fsm.NewFSM(
		StateTransferLeaderBegin,
		transferLeaderEvents,
		transferLeaderCallbacks,
	)

	return &TransferLeaderProcedure{fsm: transferLeaderOperationFsm, dispatch: dispatch, cluster: cluster, id: id, state: StateInit, oldLeader: oldLeader, newLeader: newLeader}
}

func (p *TransferLeaderProcedure) ID() uint64 {
	return p.id
}

func (p *TransferLeaderProcedure) Typ() Typ {
	return TransferLeader
}

func (p *TransferLeaderProcedure) Start(ctx context.Context) error {
	p.UpdateStateWithLock(StateRunning)

	transferLeaderRequest := &TransferLeaderCallbackRequest{
		cluster:   p.cluster,
		cxt:       ctx,
		newLeader: p.newLeader,
		oldLeader: p.oldLeader,
		dispatch:  p.dispatch,
	}

	if err := p.fsm.Event(EventTransferLeaderPrepare, transferLeaderRequest); err != nil {
		err := p.fsm.Event(EventTransferLeaderFailed, transferLeaderRequest)
		p.UpdateStateWithLock(StateFailed)
		return errors.WithMessage(err, "coordinator transferLeaderShard start")
	}

	if err := p.fsm.Event(EventTransferLeaderSuccess, transferLeaderRequest); err != nil {
		return errors.WithMessage(err, "coordinator transferLeaderShard start")
	}

	p.UpdateStateWithLock(StateFinished)
	return nil
}

func (p *TransferLeaderProcedure) Cancel(_ context.Context) error {
	p.UpdateStateWithLock(StateCancelled)
	return nil
}

func (p *TransferLeaderProcedure) State() State {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.state
}

func transferLeaderPrepareCallback(event *fsm.Event) {
	request := event.Args[0].(*TransferLeaderCallbackRequest)
	cxt := request.cxt

	closeShardAction := dispatch.CloseShardAction{
		ShardIDs: []uint32{request.oldLeader.Id},
	}
	if err := request.dispatch.CloseShards(cxt, request.oldLeader.Node, closeShardAction); err != nil {
		event.Cancel(errors.WithMessage(err, "coordinator transferLeaderShard prepare callback"))
		return
	}

	openShardAction := dispatch.OpenShardAction{
		ShardIDs: []uint32{request.newLeader.Id},
	}
	if err := request.dispatch.OpenShards(cxt, request.newLeader.Node, openShardAction); err != nil {
		event.Cancel(errors.WithMessage(err, "coordinator transferLeaderShard prepare callback"))
		return
	}
}

func transferLeaderFailedCallback(_ *fsm.Event) {
	// TODO: Use RollbackProcedure to rollback transfer failed
}

func transferLeaderSuccessCallback(event *fsm.Event) {
	request := event.Args[0].(*TransferLeaderCallbackRequest)
	c := request.cluster
	ctx := request.cxt

	// Update cluster topology
	shardView, err := c.GetClusterShardView()
	if err != nil {
		event.Cancel(errors.WithMessage(err, "TransferLeaderProcedure success callback"))
		return
	}
	var oldLeaderIndex int
	for i := 0; i < len(shardView); i++ {
		shardID := shardView[i].Id
		if shardID == request.oldLeader.Id {
			oldLeaderIndex = i
		}
	}
	shardView = append(shardView[:oldLeaderIndex], shardView[oldLeaderIndex+1:]...)
	shardView = append(shardView, request.newLeader)

	if err := c.UpdateClusterTopology(ctx, c.GetClusterState(), shardView); err != nil {
		event.Cancel(errors.WithMessage(err, "TransferLeaderProcedure start success callback"))
		return
	}
}

func (p *TransferLeaderProcedure) UpdateStateWithLock(state State) {
	p.lock.Lock()
	p.state = state
	p.lock.Unlock()
}
