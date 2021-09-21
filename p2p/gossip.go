package p2p

import (
	"context"

	"go.uber.org/multierr"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/celestiaorg/optimint/log"
)

// GossipMessage represents message gossiped via P2P network (e.g. transaction, Block etc).
type GossipMessage struct {
	Data []byte
	From peer.ID
}

// GossipHandler is a callback function type.
type GossipHandler func(*GossipMessage)

// GossiperOption sets optional parameters of Gossiper.
type GossiperOption func(*Gossiper) error

// WithHandler option sets message handler to Gossiper.
func WithHandler(handler GossipHandler) GossiperOption {
	return func(g *Gossiper) error {
		g.handler = handler
		return nil
	}
}

// WithValidator options registers topic validator for Gossiper.
func WithValidator(validator pubsub.Validator) GossiperOption {
	return func(g *Gossiper) error {
		return g.ps.RegisterTopicValidator(g.topic.String(), validator)
	}
}

// Gossiper is an abstraction of P2P publish subscribe mechanism.
type Gossiper struct {
	ownId peer.ID

	ps      *pubsub.PubSub
	topic   *pubsub.Topic
	sub     *pubsub.Subscription
	handler GossipHandler

	logger log.Logger
}

// NewGossiper creates new, ready to use instance of Gossiper.
//
// Returned Gossiper object can be used for sending (Publishing) and receiving messages in topic identified by topicStr.
func NewGossiper(host host.Host, ps *pubsub.PubSub, topicStr string, logger log.Logger, options ...GossiperOption) (*Gossiper, error) {
	topic, err := ps.Join(topicStr)
	if err != nil {
		return nil, err
	}

	subscription, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}
	g := &Gossiper{
		ownId:   host.ID(),
		ps:      ps,
		topic:   topic,
		sub:     subscription,
		handler: nil,
		logger:  logger,
	}

	for _, option := range options {
		err := option(g)
		if err != nil {
			return nil, err
		}
	}

	return g, nil
}

func (g *Gossiper) Close() error {
	err := g.ps.UnregisterTopicValidator(g.topic.String())
	g.sub.Cancel()
	return multierr.Combine(
		err,
		g.topic.Close(),
	)
}

// Publish publishes data to gossip topic.
func (g *Gossiper) Publish(ctx context.Context, data []byte) error {
	return g.topic.Publish(ctx, data)
}

// ProcessMessages waits for messages published in the topic and execute handler.
func (g *Gossiper) ProcessMessages(ctx context.Context) {
	for {
		msg, err := g.sub.Next(ctx)
		if err != nil {
			g.logger.Error("failed to read message", "error", err)
			return
		}
		if msg.GetFrom() == g.ownId {
			continue
		}

		if g.handler != nil {
			g.handler(&GossipMessage{Data: msg.Data, From: msg.GetFrom()})
		}
	}
}
