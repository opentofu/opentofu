package plugin

import (
	"context"
	"crypto/md5"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

type pluginClient struct {
	provider  providers.Interface
	cfgType   string
	chunkSize int64

	workspace string
}

func (p *pluginClient) Workspace(ws string) *pluginClient {
	return &pluginClient{
		provider:  p.provider,
		cfgType:   p.cfgType,
		chunkSize: p.chunkSize,
		workspace: ws,
	}
}

func (p *pluginClient) Get(_ context.Context) (*remote.Payload, error) {
	resp := p.provider.ReadStateBytes(context.TODO(), providers.ReadStateBytesRequest{
		TypeName: p.cfgType,
		StateId:  p.workspace,
	})

	var result remote.Payload

	for chunk := range resp {
		// TODO safer byte ranges + prealloc
		result.Data = append(result.Data, chunk.Bytes...)
		if chunk.Diagnostics.HasErrors() {
			return nil, chunk.Diagnostics.Err()
		}
	}

	if len(result.Data) == 0 {
		return nil, nil
	}

	// Generate the MD5
	hash := md5.Sum(result.Data)
	result.MD5 = hash[:] // Is this ever used?

	return &result, nil
}

func (p *pluginClient) Put(_ context.Context, data []byte) error {
	resp := p.provider.WriteStateBytes(context.TODO(), func(yield func(providers.WriteStateBytesRequest) bool) {
		chunkStart := int64(0)
		size := int64(len(data))

		meta := &providers.WriteStateRequestChunkMeta{
			TypeName: p.cfgType,
			StateId:  p.workspace,
		}

		for chunkStart < size {
			chunkEnd := chunkStart + p.chunkSize
			if chunkEnd > size {
				chunkEnd = size
			}
			chunk := providers.WriteStateBytesRequest{
				Meta: meta,
				StateByteChunk: providers.StateByteChunk{
					Bytes:       data[chunkStart:chunkEnd],
					TotalLength: size, // TODO why is this here and not declared up front?  Perhaps for situations where it's unknown?
					Range: providers.StateByteRange{
						Start: chunkStart,
						End:   chunkEnd,
					},
				},
			}
			meta = nil
			if !yield(chunk) {
				return
			}
			chunkStart += p.chunkSize
		}

	})

	return resp.Diagnostics.Err()
}

func (p *pluginClient) Delete(_ context.Context) error {
	resp := p.provider.DeleteState(context.TODO(), providers.DeleteStateRequest{
		TypeName: p.cfgType,
		StateId:  p.workspace,
	})
	return resp.Diagnostics.Err()
}

func (p *pluginClient) Lock(_ context.Context, info *statemgr.LockInfo) (string, error) {
	lockResult := p.provider.LockState(context.TODO(), providers.LockStateRequest{
		TypeName:  p.cfgType,
		StateId:   p.workspace,
		Operation: info.Operation,
	})

	return lockResult.LockId, lockResult.Diagnostics.Err()
}

func (p *pluginClient) Unlock(_ context.Context, id string) error {
	unlockResult := p.provider.UnlockState(context.TODO(), providers.UnlockStateRequest{
		TypeName: p.cfgType,
		StateId:  p.workspace,
		LockId:   id,
	})

	return unlockResult.Diagnostics.Err()
}
