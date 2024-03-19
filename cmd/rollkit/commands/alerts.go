package commands

import (
	"encoding/json"
	"io"
	"net/http"

	rpcjson "github.com/rollkit/rollkit/rpc/json"
	"github.com/rollkit/rollkit/types"
	"github.com/spf13/cobra"
)

// AlertsCmd is the command to show the alerts for the node
var AlertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Show the alerts for the node",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create an HTTP client
		client := &http.Client{}

		// Make an HTTP GET request
		// TODO: As we extend the CLI we can refactor this to be more generic
		resp, err := client.Get("http://127.0.0.1:26657/alerts")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		// Unmarshal the alerts
		var alerts rpcjson.AlertsResponse
		json.Unmarshal(body, &alerts)

		// Use the helper functin to print the alerts
		types.PrintAlerts(alerts.CriticalAlerts, types.SeverityCritical)
		types.PrintAlerts(alerts.ErrorAlerts, types.SeverityError)
		types.PrintAlerts(alerts.WarningAlerts, types.SeverityWarning)
		types.PrintAlerts(alerts.InfoAlerts, types.SeverityInfo)
		return nil
	},
}
