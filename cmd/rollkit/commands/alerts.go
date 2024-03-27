package commands

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	rpcjson "github.com/rollkit/rollkit/rpc/json"
	"github.com/rollkit/rollkit/types"
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
		defer func() {
			// Ignoring error as there isn't anything to do with it
			// as the program exits
			_ = resp.Body.Close()
		}()

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		// Unmarshal the alerts
		var alerts rpcjson.AlertsResponse
		err = json.Unmarshal(body, &alerts)
		if err != nil {
			return err
		}

		// Use the helper functin to print the alerts
		err = types.PrintAlerts(alerts.CriticalAlerts, types.SeverityCritical)
		err = errors.Join(err, types.PrintAlerts(alerts.ErrorAlerts, types.SeverityError))
		err = errors.Join(err, types.PrintAlerts(alerts.WarningAlerts, types.SeverityWarning))
		err = errors.Join(err, types.PrintAlerts(alerts.InfoAlerts, types.SeverityInfo))
		return err
	},
}
