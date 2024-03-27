package block

import "github.com/rollkit/rollkit/types"

// Alerts implements the types.Alerter interface for the block manager. It
// returns the alerts registered by the manager.
func (m *Manager) Alerts() (crit, err, warn, info []types.Alert) {
	return m.alerter.Alerts()
}
