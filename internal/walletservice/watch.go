package walletservice

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/report"
	"github.com/ethereum/go-ethereum/core/types"
)

const watchBackoff = time.Second

// StartWatch subscribes to new heads and pushes account snapshots to the UI.
//
//wails:ignore
func (s *WalletService) StartWatch(ctx context.Context) {
	go s.watch(ctx)
}

func (s *WalletService) watch(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.watchOnce(ctx); err != nil && ctx.Err() == nil {
			if errors.Is(err, chain.ErrAnvilUnavailable) {
				// Dialog every backoff would spam while Anvil is down.
				log.Print(err)
			} else {
				report.Error(err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(watchBackoff):
			}
			continue
		}
		return
	}
}

func (s *WalletService) watchOnce(ctx context.Context) error {
	if err := s.catchUp(ctx); err != nil {
		report.Error(err)
	}
	s.emitTransactions()

	client, err := chain.ConnectURL(ctx, chain.WebsocketURL(s.url))
	if err != nil {
		return err
	}
	defer client.Close()

	heads := make(chan *types.Header)
	// eth_subscribe "newHeads" — Anvil-only: local Foundry WebSocket, not HTTP JSON-RPC.
	sub, err := client.SubscribeNewHeads(ctx, heads)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	return s.watchHeads(ctx, heads, sub.Err())
}

func (s *WalletService) watchHeads(ctx context.Context, heads <-chan *types.Header, errc <-chan error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-errc:
			if !ok {
				return nil
			}
			if err != nil {
				return err
			}
		case head, ok := <-heads:
			if !ok {
				return nil
			}
			if head != nil && head.Number != nil {
				if err := s.ingestThrough(ctx, head.Number.Uint64()); err != nil {
					report.Error(err)
				}
				s.emitTransactions()
			}
			accs, err := s.Accounts()
			if err != nil {
				report.Error(err)
				continue
			}
			s.mu.Lock()
			emit := s.emit
			s.mu.Unlock()
			if emit != nil {
				emit("accounts", accs)
			}
		}
	}
}
