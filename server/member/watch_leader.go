// Copyright 2022 CeresDB Project Authors. Licensed under Apache-2.0.

package member

import (
	"context"
	"time"

	"github.com/CeresDB/ceresmeta/pkg/log"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

const (
	WatchLeaderFailInterval = time.Duration(200) * time.Millisecond

	waitReasonFailEtcd    = "fail to access etcd"
	waitReasonResetLeader = "leader is reset"
	waitReasonElectLeader = "leader is electing"
	waitReasonNoWait      = ""
)

type WatchContext interface {
	EtcdLeaderID() uint64
	ShouldStop() bool
	NewLease() clientv3.Lease
	NewWatcher() clientv3.Watcher
}

type LeaderWatcher struct {
	watchCtx    WatchContext
	self        *Member
	leaseTTLSec int64
}

func NewLeaderWatcher(ctx WatchContext, self *Member, leaseTTLSec int64) *LeaderWatcher {
	return &LeaderWatcher{
		ctx,
		self,
		leaseTTLSec,
	}
}

func (l *LeaderWatcher) Watch(ctx context.Context) {
	var wait string
	logger := log.With(zap.String("self", l.self.Name))

	for {
		if l.watchCtx.ShouldStop() {
			logger.Warn("stop watching leader because of server is closed")
			return
		}

		select {
		case <-ctx.Done():
			logger.Warn("stop watching leader because ctx is done")
			return
		default:
		}

		if wait != waitReasonNoWait {
			logger.Warn("sleep a while during watch", zap.String("wait-reason", wait))
			time.Sleep(WatchLeaderFailInterval)
			wait = waitReasonNoWait
		}

		// check whether leader exists.
		leaderResp, err := l.self.GetLeader(ctx)
		if err != nil {
			logger.Error("fail to get leader", zap.Error(err))
			wait = waitReasonFailEtcd
			continue
		}

		etcdLeaderID := l.watchCtx.EtcdLeaderID()
		if leaderResp.Leader == nil {
			// Leader does not exist.
			// A new leader should be elected and the etcd leader should be made the new leader.
			if l.self.ID == etcdLeaderID {
				// campaign the leader and block until leader changes.
				rawLease := l.watchCtx.NewLease()
				if err := l.self.CampaignAndKeepLeader(ctx, rawLease, l.leaseTTLSec); err != nil {
					logger.Error("fail to campaign and keep leader", zap.Error(err))
					wait = waitReasonFailEtcd
				} else {
					logger.Info("stop keeping leader")
				}
				continue
			}

			// for other nodes that is not etcd leader, just wait for the new leader elected.
			wait = waitReasonElectLeader
		} else {
			// Leader does exist.
			// A new leader should be elected (the leader should be reset by the current leader itself) if the leader is
			// not the etcd leader.
			if etcdLeaderID == leaderResp.Leader.Id {
				// watch the leader and block until leader changes.
				watcher := l.watchCtx.NewWatcher()
				l.self.WaitForLeaderChange(ctx, watcher, leaderResp.Revision)
				logger.Warn("leader changes and stop watching")
				continue
			}

			// the leader is not etcd leader and this node is leader so reset it.
			if leaderResp.Leader.Id == l.self.ID {
				if err := l.self.ResetLeader(ctx); err != nil {
					logger.Error("fail to reset leader", zap.Error(err))
					wait = waitReasonFailEtcd
				}
				continue
			}

			// the leader is not etcd leader and this node is not the leader so just wait a moment and check leader again.
			wait = waitReasonResetLeader
		}
	}
}
