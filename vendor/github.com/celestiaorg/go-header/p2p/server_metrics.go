package p2p

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	headersServedKey = "num_headers_served"
	failedRequestKey = "failed_request"
)

type serverMetrics struct {
	headersServedInst  metric.Int64Counter
	headServeTimeInst  metric.Float64Histogram
	rangeServeTimeInst metric.Float64Histogram
	getServeTimeInst   metric.Float64Histogram
}

func newServerMetrics() (m *serverMetrics, err error) {
	m = new(serverMetrics)
	m.headersServedInst, err = meter.Int64Counter(
		"hdr_p2p_exch_srvr_headers_served_counter",
		metric.WithDescription("number of headers served"),
	)
	if err != nil {
		return nil, err
	}
	m.headServeTimeInst, err = meter.Float64Histogram(
		"hdr_p2p_exch_srvr_head_serve_time_hist",
		metric.WithDescription("exchange server head serve time in seconds"),
	)
	if err != nil {
		return nil, err
	}
	m.rangeServeTimeInst, err = meter.Float64Histogram(
		"hdr_p2p_exch_srvr_range_serve_time_hist",
		metric.WithDescription("exchange server range serve time in seconds"),
	)
	if err != nil {
		return nil, err
	}
	m.getServeTimeInst, err = meter.Float64Histogram(
		"hdr_p2p_exch_srvr_get_serve_time_hist",
		metric.WithDescription("exchange server get serve time in seconds"),
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (m *serverMetrics) headServed(ctx context.Context, duration time.Duration, failed bool) {
	m.observe(ctx, func(ctx context.Context) {
		m.headersServedInst.Add(ctx,
			1,
			metric.WithAttributes(attribute.Bool(failedRequestKey, failed)),
		)
		m.headServeTimeInst.Record(ctx,
			duration.Seconds(),
			metric.WithAttributes(attribute.Bool(failedRequestKey, failed)),
		)
	})
}

func (m *serverMetrics) rangeServed(ctx context.Context, duration time.Duration, headersServed int, failed bool) {
	m.observe(ctx, func(ctx context.Context) {
		m.headersServedInst.Add(ctx,
			int64(headersServed),
			metric.WithAttributes(attribute.Bool(failedRequestKey, failed)),
		)
		m.rangeServeTimeInst.Record(ctx,
			duration.Seconds(),
			metric.WithAttributes(attribute.Int(headersServedKey, headersServed/100)), // divide by 100 to reduce cardinality
			metric.WithAttributes(attribute.Bool(failedRequestKey, failed)),
		)
	})
}

func (m *serverMetrics) getServed(ctx context.Context, duration time.Duration, failed bool) {
	m.observe(ctx, func(ctx context.Context) {
		m.headersServedInst.Add(ctx,
			1,
			metric.WithAttributes(attribute.Bool(failedRequestKey, failed)),
		)
		m.getServeTimeInst.Record(ctx,
			duration.Seconds(),
			metric.WithAttributes(attribute.Bool(failedRequestKey, failed)),
		)
	})
}

func (m *serverMetrics) observe(ctx context.Context, f func(context.Context)) {
	if m == nil {
		return
	}

	if ctx.Err() != nil {
		ctx = context.Background()
	}

	f(ctx)
}
