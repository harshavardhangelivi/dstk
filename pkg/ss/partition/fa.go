package partition

import (
	"github.com/anujga/dstk/pkg/ss/common"
	"go.uber.org/zap"
	"reflect"
)

type followingActor struct {
	actorBase
}

func (fa *followingActor) become() error {
	fa.setState(Follower)
	fa.logger.Info("became", zap.String("smstate", fa.getState().String()), zap.Int64("id", fa.id))
	for m := range fa.mailBox {
		switch m.(type) {
		case *BecomePrimary:
			pa := &primaryActor{fa.actorBase}
			return pa.become()
		case common.ClientMsg:
			// we needn't handle this because primary would've updated the state.
			// todo what should we do when primary is in different node?
		default:
			fa.logger.Warn("not handled", zap.Any("state", fa.smState), zap.Any("type", reflect.TypeOf(m)))
		}
	}
	fa.setState(Completed)
	return nil
}