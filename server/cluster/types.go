// Copyright 2022 CeresDB Project Authors. Licensed under Apache-2.0.

package cluster

import (
	"github.com/CeresDB/ceresdbproto/pkg/clusterpb"
	"github.com/CeresDB/ceresdbproto/pkg/metaservicepb"
)

type TableInfo struct {
	ID         uint64
	Name       string
	SchemaID   uint32
	SchemaName string
}

type ShardTables struct {
	Shard  *ShardInfo
	Tables []*TableInfo
}

type ShardInfo struct {
	ShardID   uint32
	ShardRole clusterpb.ShardRole
	Version   uint64
}

type NodeShard struct {
	Endpoint  string
	ShardInfo *ShardInfo
}

type RouteEntry struct {
	Table      *TableInfo
	NodeShards []*NodeShard
}

type RouteTablesResult struct {
	Version      uint64
	RouteEntries map[string]*RouteEntry
}

type GetNodesResult struct {
	ClusterTopologyVersion uint64
	NodeShards             []*NodeShard
}

func ConvertShardsInfoToPB(shard *ShardInfo) *metaservicepb.ShardInfo {
	return &metaservicepb.ShardInfo{
		ShardId: shard.ShardID,
		Role:    shard.ShardRole,
		Version: shard.Version,
	}
}

func ConvertTableInfoToPB(table *TableInfo) *metaservicepb.TableInfo {
	return &metaservicepb.TableInfo{
		Id:         table.ID,
		Name:       table.Name,
		SchemaId:   table.SchemaID,
		SchemaName: table.SchemaName,
	}
}
