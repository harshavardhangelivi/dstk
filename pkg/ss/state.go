package ss

import (
	pb "github.com/anujga/dstk/pkg/api/proto"
	"github.com/anujga/dstk/pkg/rangemap"
	"go.uber.org/zap"
)

type appState struct {
	s interface{}
}

func (a *appState) ResponseChannel() chan interface{} {
	return nil
}

func (a *appState) State() interface{} {
	return a.s
}

type state struct {
	m            *rangemap.RangeMap
	lastModified int64
	logger       *zap.SugaredLogger
}

// path=control
func (s *state) add(p *pb.Partition, consumer ConsumerFactory, stateListener chan<- interface{}) (*PartRange, error) {
	var err error
	s.logger.Info("AddPartition Start", "part", p)
	defer s.logger.Info("AddPartition Status", "part", p, "err", err)
	c, maxOutstanding, err := consumer.Make(p)
	if err != nil {
		return nil, err
	}
	part := NewPartRange(p, c, maxOutstanding, stateListener)
	if err = s.m.Put(part); err != nil {
		return nil, err
	}
	part.Run()
	return part, nil
}