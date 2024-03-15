package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/tabwriter"
)

// The following consts are the different types of severity levels available in
// the alert system.
const (
	// SeverityUnknown is the value of an uninitialized severity and should never
	// be used.
	SeverityUnknown = iota
	// SeverityInfo shows the user potentially useful information, such as the
	// status of long running actions.
	SeverityInfo
	// SeverityWarning warns the user about potential issues which might require
	// preventive actions.
	SeverityWarning
	// SeverityError should be used for information about the system where
	// immediate action is recommended to avoid further issues like data not
	// being posted to DA.
	SeverityError
	// SeverityCritical should be used for critical errors. e.g. a data not
	// being posted to DA without immediate action.
	SeverityCritical
)

// The following consts are a list of AlertIDs. All IDs used throughout rollkit
// should be unique and listed here.
const (
	// alertIDUnknown is the id of an unknown alert.
	//lint:ignore U1000 keeping for safety
	alertIDUnknown = "unknown"

	// AlertIDBlockNotSubmitted is the id of the alert that is registered if
	// there is an error while submitting a block.
	AlertIDBlockNotSubmitted = "block-not-submitted"
)

type (
	// Alerter is the interface implemented by all top-level modules. It's an
	// interface that allows for asking a module about potential issues.
	Alerter interface {
		Alerts() (crit, err, warn, info []Alert)
	}

	// Alert is a type that contains essential information about an alert.
	Alert struct {
		// Cause is the cause for the Alert.
		// e.g. "Wallet is locked"
		Cause string `json:"cause"`
		// Msg is the message the Alert is meant to convey to the user.
		// e.g. "Contractor can't form new contrats"
		Msg string `json:"msg"`
		// Module contains information about what module the alert originated from.
		Module string `json:"module"`
		// Severity categorizes the Alerts to allow for an easy way to filter them.
		Severity AlertSeverity `json:"severity"`
	}

	// AlertID is a helper type for an Alert's ID.
	AlertID string

	// AlertSeverity describes the severity of an alert.
	AlertSeverity uint64
)

// Equals returns true if x and y are identical alerts
func (x Alert) Equals(y Alert) bool {
	return x.Module == y.Module && x.Cause == y.Cause && x.Msg == y.Msg && x.Severity == y.Severity
}

// EqualsWithErrorCause returns true if x and y have the same module, message,
// and severity and if the provided error is in both of the alert's causes
func (x Alert) EqualsWithErrorCause(y Alert, causeErr string) bool {
	firstCheck := x.Module == y.Module && x.Msg == y.Msg && x.Severity == y.Severity
	causeCheck := strings.Contains(x.Cause, causeErr) && strings.Contains(y.Cause, causeErr)
	return firstCheck && causeCheck
}

// MarshalJSON defines a JSON encoding for the AlertSeverity.
func (a AlertSeverity) MarshalJSON() ([]byte, error) {
	switch a {
	case SeverityInfo:
	case SeverityWarning:
	case SeverityError:
	case SeverityCritical:
	default:
		return nil, errors.New("unknown AlertSeverity")
	}
	return json.Marshal(a.String())
}

// UnmarshalJSON attempts to decode an AlertSeverity.
func (a *AlertSeverity) UnmarshalJSON(b []byte) error {
	var severityStr string
	if err := json.Unmarshal(b, &severityStr); err != nil {
		return err
	}
	switch severityStr {
	case "info":
		*a = SeverityInfo
	case "warning":
		*a = SeverityWarning
	case "error":
		*a = SeverityError
	case "critical":
		*a = SeverityCritical
	default:
		return fmt.Errorf("unknown severity '%v'", severityStr)
	}
	return nil
}

// String converts an alertSeverity to a string
func (a AlertSeverity) String() string {
	switch a {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	case SeverityUnknown:
	default:
	}
	return "unknown"
}

// GenericAlerter implements the Alerter interface. It can be used as a helper
// type to implement the Alerter interface for modules and submodules.
type (
	GenericAlerter struct {
		alerts map[AlertID]Alert
		module string
		mu     sync.Mutex
	}
)

// NewAlerter creates a new alerter for the renter.
func NewAlerter(module string) *GenericAlerter {
	a := &GenericAlerter{
		alerts: make(map[AlertID]Alert),
		module: module,
	}
	return a
}

// Alerts returns the current alerts tracked by the alerter.
func (a *GenericAlerter) Alerts() (crit, err, warn, info []Alert) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, alert := range a.alerts {
		switch alert.Severity {
		case SeverityInfo:
			info = append(info, alert)
		case SeverityCritical:
			crit = append(crit, alert)
		case SeverityError:
			err = append(err, alert)
		case SeverityWarning:
			warn = append(warn, alert)
		default:
			panic(fmt.Sprint("Alerts: invalid severity", alert.Severity))
		}
	}
	return
}

// RegisterAlert adds an alert to the alerter.
func (a *GenericAlerter) RegisterAlert(id AlertID, msg, cause string, severity AlertSeverity) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alerts[id] = Alert{
		Cause:    cause,
		Module:   a.module,
		Msg:      msg,
		Severity: severity,
	}
}

// UnregisterAlert removes an alert from the alerter by id.
func (a *GenericAlerter) UnregisterAlert(id AlertID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.alerts, id)
}

// PrintAlerts is a helper function to print details of a slice of alerts
// with given severity description to command line
func PrintAlerts(alerts []Alert, as AlertSeverity) error {
	w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
	fmt.Fprintf(w, "\n  There are %v %s alerts\n", len(alerts), as.String())

	for _, a := range alerts {
		fmt.Fprintln(w, "------------------")
		fmt.Fprintf(w, "\tModule:\t%v\n", a.Module)
		fmt.Fprintf(w, "\tSeverity:\t%v\n", a.Severity.String())
		fmt.Fprintf(w, "\tMsg:\t%v\n", a.Msg)
		fmt.Fprintf(w, "\tCause:\t%v\n", a.Cause)
		fmt.Fprintln(w, "------------------")
	}
	return w.Flush()
}
