package connectapi

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type botAssembler struct {
	api *API
}

func newBotAssembler(api *API) *botAssembler {
	return &botAssembler{api: api}
}

// assemble hydrates optional credential-use telemetry only for bots selected
// for the response. Independent per-bot reads use bounded concurrency.
func (a *botAssembler) assemble(ctx context.Context, bots []*core.Bot) ([]*apiv1.Bot, error) {
	return parallel.Map(ctx, maxConnectAPIHydrationConcurrency, bots, func(ctx context.Context, _ int, bot *core.Bot) (*apiv1.Bot, error) {
		a.api.core.HydrateBotCredentialUsage(ctx, bot)
		return apiBot(ctx, a.api, bot)
	})
}

func (a *botAssembler) assembleOne(ctx context.Context, bot *core.Bot) (*apiv1.Bot, error) {
	bots, err := a.assemble(ctx, []*core.Bot{bot})
	if err != nil {
		return nil, err
	}
	if len(bots) != 1 {
		return nil, fmt.Errorf("bot assembler returned %d bots, want 1", len(bots))
	}
	return bots[0], nil
}
