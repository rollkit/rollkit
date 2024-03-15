package types

import (
	"encoding/json"
	"strconv"
	"testing"
)

// TestMarshalUnmarshalAlertSeverity tests the custom marshaling/unmarshaling
// code for AlertSeverity.
func TestMarshalUnmarshalAlertSeverity(t *testing.T) {
	severityUnknown := AlertSeverity(SeverityUnknown)
	severityWarning := AlertSeverity(SeverityWarning)
	severityError := AlertSeverity(SeverityError)
	severityCritical := AlertSeverity(SeverityCritical)
	severityInvalid := AlertSeverity(42)

	var s AlertSeverity
	// Marshal/Unmarshal unknown.
	_, err := json.Marshal(severityUnknown)
	if err == nil {
		t.Fatal("Shouldn't be able to marshal unknown")
	}
	// Marshal/Unmarshal critical.
	b, err := json.Marshal(severityCritical)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s != SeverityCritical {
		t.Fatal("result not the same severity as input")
	}
	// Marshal/Unmarshal warning.
	b, err = json.Marshal(severityWarning)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s != SeverityWarning {
		t.Fatal("result not the same severity as input")
	}
	// Marshal/Unmarshal error.
	b, err = json.Marshal(severityError)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s != SeverityError {
		t.Fatal("result not the same severity as input")
	}
	// Marshal/Unmarshal invalid.
	_, err = json.Marshal(severityInvalid)
	if err == nil {
		t.Fatal("Shouldn't be able to marshal invalid")
	}
}

// TestAlerter tests the following:
//   - alerts can be registered
//   - the return values contain the right alerts
//   - alerts can be unregisterd
func TestAlerter(t *testing.T) {
	alerter := NewAlerter(t.Name())

	// Create 20 alert IDs
	alertIDs := make([]AlertID, 20)
	for i := 0; i < 20; i++ {
		alertIDs[i] = AlertID(strconv.Itoa(i))
	}
	// Register the 20 alerts, 5 of each severity
	for i, id := range alertIDs {
		alerter.RegisterAlert(id, "msg"+string(id), "cause"+string(id), AlertSeverity(i%4+1))
	}

	// Verify the correct number of alerts by severity
	crit, err, warn, info := alerter.Alerts()
	if len(crit) != 5 || len(err) != 5 || len(warn) != 5 || len(info) != 5 {
		t.Fatalf("returned slices have wrong lengths %v %v %v %v", len(crit), len(err), len(warn), len(info))
	}

	// Verify that the alerts are sorted properly by severity
	for _, alert := range crit {
		if alert.Severity != SeverityCritical {
			t.Fatal("alert has wrong severity")
		}
	}
	for _, alert := range err {
		if alert.Severity != SeverityError {
			t.Fatal("alert has wrong severity")
		}
	}
	for _, alert := range warn {
		if alert.Severity != SeverityWarning {
			t.Fatal("alert has wrong severity")
		}
	}
	for _, alert := range info {
		if alert.Severity != SeverityInfo {
			t.Fatal("alert has wrong severity")
		}
	}

	// Unregister the alerts
	for _, id := range alertIDs {
		alerter.UnregisterAlert(id)
	}

	// Verfiy that the alerts are unregistered
	crit, err, warn, info = alerter.Alerts()
	if len(crit) != 0 || len(err) != 0 || len(warn) != 0 || len(info) != 0 {
		t.Fatalf("returned slices have wrong lengths %v %v %v %v", len(crit), len(err), len(warn), len(info))
	}
}
