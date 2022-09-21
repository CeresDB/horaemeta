package procedure

import (
	"context"
)

func NewSwitchLeaderProcedure() *SwitchLeaderProcedure {
	return nil
}

func LoadSwitchLeaderProcedure(_ []byte) *SwitchLeaderProcedure {
	return nil
}

// nolint
type SwitchLeaderProcedure struct {
	writer Write
}

func (s *SwitchLeaderProcedure) ID() uint64 {
	return 0
}

func (s *SwitchLeaderProcedure) Type() procedureType {
	return switchLeader
}

func (s *SwitchLeaderProcedure) Start(_ context.Context) error {
	return nil
}

func (s *SwitchLeaderProcedure) Cancel(_ context.Context) error {
	return nil
}

func (s *SwitchLeaderProcedure) State() State {
	return ""
}
