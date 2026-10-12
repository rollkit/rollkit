package store

import (
	"context"
	"encoding/hex"

	ds "github.com/ipfs/go-datastore"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/evstack/ev-node/types"
)

var _ Store = (*tracedStore)(nil)

type tracedStore struct {
	inner  Store
	tracer trace.Tracer
}

// WithTracingStore wraps a Store with OpenTelemetry tracing.
func WithTracingStore(inner Store) Store {
	return &tracedStore{
		inner:  inner,
		tracer: otel.Tracer("ev-node/store"),
	}
}

func (t *tracedStore) Height(ctx context.Context) (uint64, error) {
	ctx, span := t.tracer.Start(ctx, "Store.Height")
	defer span.End()

	height, err := t.inner.Height(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return height, err
	}

	span.SetAttributes(attribute.Int64("height", int64(height)))
	return height, nil
}

func (t *tracedStore) GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
	ctx, span := t.tracer.Start(ctx, "Store.GetBlockData",
		trace.WithAttributes(attribute.Int64("height", int64(height))),
	)
	defer span.End()

	header, data, err := t.inner.GetBlockData(ctx, height)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return header, data, err
	}

	return header, data, nil
}

func (t *tracedStore) GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error) {
	ctx, span := t.tracer.Start(ctx, "Store.GetBlockByHash",
		trace.WithAttributes(attribute.String("hash", hex.EncodeToString(hash))),
	)
	defer span.End()

	header, data, err := t.inner.GetBlockByHash(ctx, hash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return header, data, err
	}

	if header != nil {
		span.SetAttributes(attribute.Int64("height", int64(header.Height())))
	}
	return header, data, nil
}

func (t *tracedStore) GetSignature(ctx context.Context, height uint64) (*types.Signature, error) {
	ctx, span := t.tracer.Start(ctx, "Store.GetSignature",
		trace.WithAttributes(attribute.Int64("height", int64(height))),
	)
	defer span.End()

	sig, err := t.inner.GetSignature(ctx, height)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return sig, err
	}

	return sig, nil
}

func (t *tracedStore) GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error) {
	ctx, span := t.tracer.Start(ctx, "Store.GetSignatureByHash",
		trace.WithAttributes(attribute.String("hash", hex.EncodeToString(hash))),
	)
	defer span.End()

	sig, err := t.inner.GetSignatureByHash(ctx, hash)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return sig, err
	}

	return sig, nil
}

func (t *tracedStore) GetHeader(ctx context.Context, height uint64) (*types.SignedHeader, error) {
	ctx, span := t.tracer.Start(ctx, "Store.GetHeader",
		trace.WithAttributes(attribute.Int64("height", int64(height))),
	)
	defer span.End()

	header, err := t.inner.GetHeader(ctx, height)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return header, err
	}

	return header, nil
}

func (t *tracedStore) GetState(ctx context.Context) (types.State, error) {
	ctx, span := t.tracer.Start(ctx, "Store.GetState")
	defer span.End()

	state, err := t.inner.GetState(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return state, err
	}

	span.SetAttributes(attribute.Int64("state.height", int64(state.LastBlockHeight)))
	return state, nil
}

func (t *tracedStore) GetStateAtHeight(ctx context.Context, height uint64) (types.State, error) {
	ctx, span := t.tracer.Start(ctx, "Store.GetStateAtHeight",
		trace.WithAttributes(attribute.Int64("height", int64(height))),
	)
	defer span.End()

	state, err := t.inner.GetStateAtHeight(ctx, height)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return state, err
	}

	return state, nil
}

func (t *tracedStore) GetMetadata(ctx context.Context, key string) ([]byte, error) {
	ctx, span := t.tracer.Start(ctx, "Store.GetMetadata",
		trace.WithAttributes(attribute.String("key", key)),
	)
	defer span.End()

	data, err := t.inner.GetMetadata(ctx, key)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return data, err
	}

	span.SetAttributes(attribute.Int("value.size", len(data)))
	return data, nil
}

func (t *tracedStore) SetMetadata(ctx context.Context, key string, value []byte) error {
	ctx, span := t.tracer.Start(ctx, "Store.SetMetadata",
		trace.WithAttributes(
			attribute.String("key", key),
			attribute.Int("value.size", len(value)),
		),
	)
	defer span.End()

	err := t.inner.SetMetadata(ctx, key, value)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (t *tracedStore) DeleteMetadata(ctx context.Context, key string) error {
	ctx, span := t.tracer.Start(ctx, "Store.DeleteMetadata",
		trace.WithAttributes(
			attribute.String("key", key),
		),
	)
	defer span.End()

	err := t.inner.DeleteMetadata(ctx, key)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (t *tracedStore) BatchMetadata(ctx context.Context, puts []MetadataKV, deletes []string) error {
	ctx, span := t.tracer.Start(ctx, "Store.BatchMetadata",
		trace.WithAttributes(
			attribute.Int("puts", len(puts)),
			attribute.Int("deletes", len(deletes)),
		),
	)
	defer span.End()

	err := t.inner.BatchMetadata(ctx, puts, deletes)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (t *tracedStore) PruneStates(ctx context.Context, height uint64) error {
	ctx, span := t.tracer.Start(ctx, "Store.PruneStates",
		trace.WithAttributes(attribute.Int64("height", int64(height))),
	)
	defer span.End()

	if err := t.inner.PruneStates(ctx, height); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (t *tracedStore) DeleteStateAtHeight(ctx context.Context, height uint64) error {
	ctx, span := t.tracer.Start(ctx, "Store.DeleteStateAtHeight",
		trace.WithAttributes(attribute.Int64("height", int64(height))),
	)
	defer span.End()

	if err := t.inner.DeleteStateAtHeight(ctx, height); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (t *tracedStore) Rollback(ctx context.Context, height uint64, aggregator bool) error {
	ctx, span := t.tracer.Start(ctx, "Store.Rollback",
		trace.WithAttributes(
			attribute.Int64("height", int64(height)),
			attribute.Bool("aggregator", aggregator),
		),
	)
	defer span.End()

	err := t.inner.Rollback(ctx, height, aggregator)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (t *tracedStore) PruneBlocks(ctx context.Context, height uint64) error {
	ctx, span := t.tracer.Start(ctx, "Store.PruneBlocks",
		trace.WithAttributes(attribute.Int64("height", int64(height))),
	)
	defer span.End()

	err := t.inner.PruneBlocks(ctx, height)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (t *tracedStore) Close() error {
	return t.inner.Close()
}

func (t *tracedStore) NewBatch(ctx context.Context) (Batch, error) {
	ctx, span := t.tracer.Start(ctx, "Store.NewBatch")
	defer span.End()

	batch, err := t.inner.NewBatch(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &tracedBatch{
		inner:  batch,
		tracer: t.tracer,
		ctx:    ctx,
	}, nil
}

var _ Batch = (*tracedBatch)(nil)

type tracedBatch struct {
	inner  Batch
	tracer trace.Tracer
	ctx    context.Context
}

func (b *tracedBatch) SaveBlockData(header *types.SignedHeader, data *types.Data, signature *types.Signature) error {
	_, span := b.tracer.Start(b.ctx, "Batch.SaveBlockData",
		trace.WithAttributes(attribute.Int64("height", int64(header.Height()))),
	)
	defer span.End()

	err := b.inner.SaveBlockData(header, data, signature)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (b *tracedBatch) SaveBlockDataFromBytes(header *types.SignedHeader, headerBlob, dataBlob []byte, signature *types.Signature) error {
	_, span := b.tracer.Start(b.ctx, "Batch.SaveBlockDataFromBytes",
		trace.WithAttributes(attribute.Int64("height", int64(header.Height()))),
	)
	defer span.End()

	err := b.inner.SaveBlockDataFromBytes(header, headerBlob, dataBlob, signature)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (b *tracedBatch) SetHeight(height uint64) error {
	_, span := b.tracer.Start(b.ctx, "Batch.SetHeight",
		trace.WithAttributes(attribute.Int64("height", int64(height))),
	)
	defer span.End()

	err := b.inner.SetHeight(height)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (b *tracedBatch) UpdateState(state types.State) error {
	_, span := b.tracer.Start(b.ctx, "Batch.UpdateState",
		trace.WithAttributes(attribute.Int64("state.height", int64(state.LastBlockHeight))),
	)
	defer span.End()

	err := b.inner.UpdateState(state)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (b *tracedBatch) Commit() error {
	_, span := b.tracer.Start(b.ctx, "Batch.Commit")
	defer span.End()

	err := b.inner.Commit()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (b *tracedBatch) Put(key ds.Key, value []byte) error {
	_, span := b.tracer.Start(b.ctx, "Batch.Put",
		trace.WithAttributes(
			attribute.String("key", key.String()),
			attribute.Int("value.size", len(value)),
		),
	)
	defer span.End()

	err := b.inner.Put(key, value)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

func (b *tracedBatch) Delete(key ds.Key) error {
	_, span := b.tracer.Start(b.ctx, "Batch.Delete",
		trace.WithAttributes(attribute.String("key", key.String())),
	)
	defer span.End()

	err := b.inner.Delete(key)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}
