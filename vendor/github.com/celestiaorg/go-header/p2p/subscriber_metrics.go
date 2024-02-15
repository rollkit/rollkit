package p2p

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var subsMeter = otel.Meter("header/p2p-subs/")

const (
	statusKey    = "status"
	statusAccept = "accept"
	statusIgnore = "ignore"
	statusReject = "reject"
)

type subscriberMetrics struct {
	messageNumInst  metric.Int64Counter
	messageSizeInst metric.Int64Histogram

	messageTimeLast atomic.Pointer[time.Time]
	messageTimeInst metric.Float64Histogram

	subscriptionNum     atomic.Int64
	subscriptionNumInst metric.Int64ObservableGauge
	subscriptionNumReg  metric.Registration
}

func newSubscriberMetrics() (m *subscriberMetrics, err error) {
	m = new(subscriberMetrics)
	m.messageNumInst, err = subsMeter.Int64Counter(
		"hdr_p2p_sub_msg_num_counter",
		metric.WithDescription("header message count"),
	)
	if err != nil {
		return nil, err
	}
	m.messageSizeInst, err = subsMeter.Int64Histogram(
		"hdr_p2p_sub_msg_size_hist",
		metric.WithDescription("valid header message size"),
	)
	if err != nil {
		return nil, err
	}
	m.messageTimeInst, err = subsMeter.Float64Histogram(
		"hdr_p2p_sub_msg_time_hist",
		metric.WithDescription("valid header message propagation time"),
	)
	if err != nil {
		return nil, err
	}
	m.subscriptionNumInst, err = subsMeter.Int64ObservableGauge(
		"hdr_p2p_sub_num_gauge",
		metric.WithDescription("number of active header message subscriptions"),
	)
	if err != nil {
		return nil, err
	}
	m.subscriptionNumReg, err = subsMeter.RegisterCallback(m.subscriptionCallback, m.subscriptionNumInst)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (m *subscriberMetrics) accept(ctx context.Context, size int) {
	m.observe(ctx, func(ctx context.Context) {
		m.messageNumInst.Add(ctx, 1, metric.WithAttributes(
			attribute.String(statusKey, statusAccept),
		))
		m.messageSizeInst.Record(ctx, int64(size))

		now := time.Now()
		lastTime := m.messageTimeLast.Swap(&now)
		if lastTime == nil || lastTime.IsZero() {
			return
		}
		m.messageTimeInst.Record(ctx, now.Sub(*lastTime).Seconds())
	})
}

func (m *subscriberMetrics) ignore(ctx context.Context) {
	m.observe(ctx, func(ctx context.Context) {
		m.messageNumInst.Add(ctx, 1, metric.WithAttributes(
			attribute.String(statusKey, statusIgnore),
		))
	})
}

func (m *subscriberMetrics) reject(ctx context.Context) {
	m.observe(ctx, func(ctx context.Context) {
		m.messageNumInst.Add(ctx, 1, metric.WithAttributes(
			attribute.String(statusKey, statusReject),
		))
	})
}

func (m *subscriberMetrics) subscription(num int) {
	m.observe(context.Background(), func(ctx context.Context) {
		m.subscriptionNum.Add(int64(num))
	})
}

func (m *subscriberMetrics) subscriptionCallback(_ context.Context, obs metric.Observer) error {
	obs.ObserveInt64(m.subscriptionNumInst, m.subscriptionNum.Load())
	return nil
}

func (m *subscriberMetrics) observe(ctx context.Context, observeFn func(context.Context)) {
	if m == nil {
		return
	}
	if ctx.Err() != nil {
		ctx = context.Background()
	}

	observeFn(ctx)
}

func (m *subscriberMetrics) Close() error {
	if m == nil {
		return nil
	}
	return m.subscriptionNumReg.Unregister()
}
