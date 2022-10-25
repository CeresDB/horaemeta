// Copyright 2022 CeresDB Project Authors. Licensed under Apache-2.0.

package cluster

import (
	"github.com/CeresDB/ceresdbproto/pkg/clusterpb"
)

const (
	MinShardID = 1
)

type Shard struct {
	// FIXME: The relationship between shard and node is missing here.
	meta    []*clusterpb.Shard
	nodes   []*clusterpb.Node
	tables  map[uint64]*Table // table_id => table
	version uint64
}

// / FIXME: avoid returning *clusterpb.Shard.
func (s *Shard) FindShardByNode(nodeName string) (*clusterpb.Shard, bool) {
	for i := range s.nodes {
		if s.nodes[i].GetName() == nodeName {
			shard := s.meta[i]
			return shard, true
		}
	}

	return nil, false
}

func (s *Shard) dropTableLocked(tableID uint64) {
	delete(s.tables, tableID)
}

func (s *Shard) GetLeader() *clusterpb.Shard {
	for i, shard := range s.meta {
		if clusterpb.ShardRole_LEADER == shard.ShardRole {
			return s.meta[i]
		}
	}
	return nil
}

func (s *Shard) GetVersion() uint64 {
	return s.version
}

type ShardTablesWithRole struct {
	shard  *ShardInfo
	tables []*Table
}
