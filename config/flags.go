package config

import (
	"github.com/spf13/cobra"
	"time"
)

func AddFlags(cmd *cobra.Command) {
	// TODO(tzdybal): extract default values
	cmd.Flags().Bool("optimint.aggregator", false, "run node in aggregator mode")
	cmd.Flags().String("optimint.da_layer", "mock", "Data Availability Layer Client name (mock or grpc")
	cmd.Flags().String("optimint.da_config", "", "Data Availability Layer Client config")
	cmd.Flags().Duration("optimint.block_time", 15*time.Second, "block time (for aggregator mode)")
	cmd.Flags().BytesHex("optimint.namespace_id", nil, "namespace identifies (8 bytes in hex)")
}
