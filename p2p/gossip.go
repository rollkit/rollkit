package p2p

import (
	"context"

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

// Gossiper is an abstraction of P2P publish subscribe mechanism.
type Gossiper struct {
	ownId peer.ID

	topic     *pubsub.Topic
	sub       *pubsub.Subscription
	handler   GossipHandler
	validator pubsub.Validator

	logger log.Logger
}

// NewGossip creates new, ready to use instance of Gossiper.
//
// Returned Gossiper object can be used for sending (Publishing) and receiving messages in topic identified by topicStr.
func NewGossip(host host.Host, ps *pubsub.PubSub, topicStr string, logger log.Logger) (*Gossiper, error) {
	topic, err := ps.Join(topicStr)
	if err != nil {
		return nil, err
	}

	subscription, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}
	return &Gossiper{
		ownId:   host.ID(),
		topic:   topic,
		sub:     subscription,
		handler: nil,
		logger:  logger,
	}, nil
}

func (g *Gossiper) Close() error {
	g.sub.Cancel()
	return g.topic.Close()
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
