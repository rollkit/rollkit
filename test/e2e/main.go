package main

import (
	"log"
	"time"

	"github.com/ory/dockertest"
	"google.golang.org/grpc"
)

func main() {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	net := NewNetwork(pool)

	net.SetExpire(360)

	// Build and run the given Dockerfile
	resource, err := pool.BuildAndRun("optimint/e2e-node", "/home/evan/go/src/github.com/celestiaorg/optimint/test/e2e/docker/optimint/Dockerfile", []string{})
	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}

	// insert node specifc work functions here

	// wait until the aggregator node is confirmed to have started.
	// Give up after 2 minutes
	pool.MaxWait = time.Minute * 2
	if err := pool.Retry(func() error {
		// Create a connection to the gRPC server.
		grpcConn, err := grpc.Dial(
			"localhost:9090",    // your gRPC server address.
			grpc.WithInsecure(), // The Cosmos SDK doesn't support any transport security mechanism.
		)
		defer grpcConn.Close()
		return err
	}); err != nil {
		log.Fatalf("Could not connect to aggregator: %s", err)
	}

	// When you're done, kill and remove the container
	if err = pool.Purge(resource); err != nil {
		log.Fatalf("Could not purge resource: %s", err)
	}

	// err = net.Purge()
	// if err != nil {
	// 	log.Fatal(err)
	// }

}

const (
	Agregator0 = "agregator0"
	FullNode0  = "fullnode0"
	FullNode1  = "fullnode1"
)

// Network manages multiple nodes (aka dockertest.Resources)
type Network struct {
	nodes map[string]*Node
	pool  *dockertest.Pool
}

func NewNetwork(pool *dockertest.Pool, nodes ...*Node) *Network {
	nodeMap := make(map[string]*Node)
	for _, node := range nodes {
		nodeMap[node.name] = node
	}
	return &Network{
		nodes: nodeMap,
		pool:  pool,
	}
}

func (n *Network) Purge() error {
	var returnErr error
	for name, node := range n.nodes {
		if node.resource == nil {
			continue
		}
		err := n.pool.Purge(node.resource)
		if err != nil {
			log.Printf("FAILURE TO PURGE DOCKER CONTAINER %s: %s\n", name, err)
			returnErr = err
		}
	}
	return returnErr
}

func (n *Network) SetExpire(timeLimit uint) error {
	for _, node := range n.nodes {
		err := node.resource.Expire(timeLimit)
		if err != nil {
			return err
		}
	}
	return nil
}

type Node struct {
	name       string
	runOpts    *dockertest.RunOptions
	aggregator bool
	resource   *dockertest.Resource
}

func (n *Node) setup() {
	if n.aggregator {
		n.runOpts.Env = append(n.runOpts.Env, "AGGEGATOR=true")
	}
}

func (n *Node) run(pool *dockertest.Pool) error {
	// todo(evan): maybe make this run in a goroutine
	resource, err := pool.RunWithOptions(n.runOpts)
	if err != nil {
		return err
	}
	n.resource = resource

	return nil
}
