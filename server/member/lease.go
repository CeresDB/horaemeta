package member

import (
	"context"
	"sync"
	"time"

	"github.com/CeresDB/ceresmeta/pkg/log"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

type lease struct {
	rawLease   clientv3.Lease
	rpcTimeout time.Duration
	ttlSec     int64

	// The fields below are initialized after Grant is called.
	ID     clientv3.LeaseID
	logger *zap.Logger

	expireTimeL sync.RWMutex
	expireTime  time.Time
}

func newLease(rawLease clientv3.Lease, rpcTimeout time.Duration, ttlSec int64) *lease {
	return &lease{
		rawLease:   rawLease,
		rpcTimeout: rpcTimeout,
		ttlSec:     ttlSec,
	}
}

func (l *lease) Grant(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, l.rpcTimeout)
	defer cancel()
	leaseResp, err := l.rawLease.Grant(ctx, l.ttlSec)
	if err != nil {
		return ErrGrantLease.WithCause(err)
	}

	l.ID = leaseResp.ID
	l.logger = log.With(zap.Int64("lease-id", int64(leaseResp.ID)))
	expiredAt := time.Now().Add(time.Second * time.Duration(leaseResp.TTL))
	l.setExpireTime(expiredAt)
	return nil
}

func (l *lease) Close(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, l.rpcTimeout)
	defer cancel()
	_, err := l.rawLease.Revoke(ctx, l.ID)
	return ErrRevokeLease.WithCause(err)
}

// KeepAlive renews the lease.
func (l *lease) KeepAlive(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	timeCh := l.keepAliveBg(ctx, l.rpcTimeout/3)

	var maxExpireTime time.Time
	for {
		select {
		case nextExpireTime := <-timeCh:
			if nextExpireTime.After(maxExpireTime) {
				maxExpireTime = nextExpireTime
				l.setExpireTime(maxExpireTime)
			}
		case <-time.After(l.rpcTimeout):
			l.logger.Info("lease timeout")
			return
		case <-ctx.Done():
			l.logger.Info("exit keepalive loop because ctx is done")
			return
		}
	}
}

func (l *lease) IsLeader() bool {
	return time.Now().After(l.getExpireTime())
}

func (l *lease) setExpireTime(newExpireTime time.Time) {
	l.expireTimeL.Lock()
	defer l.expireTimeL.Unlock()

	l.expireTime = newExpireTime
}

func (l *lease) getExpireTime() time.Time {
	l.expireTimeL.RLock()
	defer l.expireTimeL.RUnlock()

	return l.expireTime
}

// keepAliveBg keeps the lease alive by periodically call `lease.KeepAliveOnce` and posts back latest received expire time into the channel.
func (l *lease) keepAliveBg(ctx context.Context, interval time.Duration) <-chan time.Time {
	ch := make(chan time.Time)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		l.logger.Info("start keep lease alive", zap.Duration("interval", interval))
		defer l.logger.Info("stop keep lease alive", zap.Duration("interval", interval))

		for {
			go func() {
				start := time.Now()
				ctx1, cancel := context.WithTimeout(ctx, l.rpcTimeout)
				defer cancel()
				resp, err := l.rawLease.KeepAliveOnce(ctx1, l.ID)
				if err != nil {
					l.logger.Error("lease keep alive failed", zap.Error(err))
					return
				}
				if resp.TTL > 0 {
					expire := start.Add(time.Duration(resp.TTL) * time.Second)
					select {
					case ch <- expire:
					case <-ctx1.Done():
					}
				}
			}()

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return ch
}
