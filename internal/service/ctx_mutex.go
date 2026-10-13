package service

import (
	"context"
	"sync"
)

// ctxMutex 是可随调用方取消而放弃等待的互斥锁。调度器的写路径共用一把锁，
// 数据库卡住时，客户端已经超时放弃的 RPC 不应继续排队，把后来的请求一起拖住。
type ctxMutex struct {
	once sync.Once
	ch   chan struct{}
}

func (m *ctxMutex) init() { m.once.Do(func() { m.ch = make(chan struct{}, 1) }) }

func (m *ctxMutex) Lock(ctx context.Context) error {
	m.init()
	select {
	case m.ch <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-m.ch
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *ctxMutex) Unlock() { <-m.ch }
