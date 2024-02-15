package p2p

import (
	"context"
	"errors"
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/celestiaorg/go-header"
)

// SubscriberOption is a functional option for the Subscriber.
type SubscriberOption func(*SubscriberParams)

// SubscriberParams defines the parameters for the Subscriber
// configurable with SubscriberOption.
type SubscriberParams struct {
	networkID string
	metrics   bool
}

// Subscriber manages the lifecycle and relationship of header Module
// with the "header-sub" gossipsub topic.
type Subscriber[H header.Header[H]] struct {
	pubsubTopicID string

	metrics *subscriberMetrics
	pubsub  *pubsub.PubSub
	topic   *pubsub.Topic
	msgID   pubsub.MsgIdFunction
}

// WithSubscriberMetrics enables metrics collection for the Subscriber.
func WithSubscriberMetrics() SubscriberOption {
	return func(params *SubscriberParams) {
		params.metrics = true
	}
}

// WithSubscriberNetworkID sets the network ID for the Subscriber.
func WithSubscriberNetworkID(networkID string) SubscriberOption {
	return func(params *SubscriberParams) {
		params.networkID = networkID
	}
}

// NewSubscriber returns a Subscriber that manages the header Module's
// relationship with the "header-sub" gossipsub topic.
func NewSubscriber[H header.Header[H]](
	ps *pubsub.PubSub,
	msgID pubsub.MsgIdFunction,
	opts ...SubscriberOption,
) (*Subscriber[H], error) {
	var params SubscriberParams
	for _, opt := range opts {
		opt(&params)
	}

	var metrics *subscriberMetrics
	if params.metrics {
		var err error
		metrics, err = newSubscriberMetrics()
		if err != nil {
			return nil, err
		}
	}

	return &Subscriber[H]{
		metrics:       metrics,
		pubsubTopicID: PubsubTopicID(params.networkID),
		pubsub:        ps,
		msgID:         msgID,
	}, nil
}

// Start starts the Subscriber, registering a topic validator for the "header-sub"
// topic and joining it.
func (s *Subscriber[H]) Start(context.Context) (err error) {
	log.Infow("joining topic", "topic ID", s.pubsubTopicID)
	s.topic, err = s.pubsub.Join(s.pubsubTopicID, pubsub.WithTopicMessageIdFn(s.msgID))
	return err
}

// Stop closes the topic and unregisters its validator.
func (s *Subscriber[H]) Stop(context.Context) error {
	err := s.pubsub.UnregisterTopicValidator(s.pubsubTopicID)
	if err != nil {
		log.Warnf("unregistering validator: %s", err)
	}

	err = errors.Join(err, s.topic.Close())
	err = errors.Join(err, s.metrics.Close())
	return err
}

// SetVerifier set given verification func as Header PubSub topic validator
// Does not punish peers if *header.VerifyError is given with Uncertain set to true.
func (s *Subscriber[H]) SetVerifier(val func(context.Context, H) error) error {
	pval := func(ctx context.Context, p peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
		hdr := header.New[H]()
		err := hdr.UnmarshalBinary(msg.Data)
		if err != nil {
			log.Errorw("unmarshalling header",
				"from", p.ShortString(),
				"err", err)
			s.metrics.reject(ctx)
			return pubsub.ValidationReject
		}
		// ensure header validity
		err = hdr.Validate()
		if err != nil {
			log.Errorw("invalid header",
				"from", p.ShortString(),
				"err", err)
			s.metrics.reject(ctx)
			return pubsub.ValidationReject
		}

		var verErr *header.VerifyError
		err = val(ctx, hdr)
		switch {
		case errors.As(err, &verErr) && verErr.SoftFailure:
			s.metrics.ignore(ctx)
			return pubsub.ValidationIgnore
		case err != nil:
			s.metrics.reject(ctx)
			return pubsub.ValidationReject
		default:
		}

		// keep the valid header in the msg so Subscriptions can access it without
		// additional unmarshalling
		msg.ValidatorData = hdr
		s.metrics.accept(ctx, len(msg.Data))
		return pubsub.ValidationAccept
	}

	return s.pubsub.RegisterTopicValidator(s.pubsubTopicID, pval)
}

// Subscribe returns a new subscription to the Subscriber's
// topic.
func (s *Subscriber[H]) Subscribe() (header.Subscription[H], error) {
	if s.topic == nil {
		return nil, fmt.Errorf("header topic is not instantiated, service must be started before subscribing")
	}

	return newSubscription[H](s.topic, s.metrics)
}

// Broadcast broadcasts the given Header to the topic.
func (s *Subscriber[H]) Broadcast(ctx context.Context, header H, opts ...pubsub.PubOpt) error {
	bin, err := header.MarshalBinary()
	if err != nil {
		return err
	}
	return s.topic.Publish(ctx, bin, opts...)
}
