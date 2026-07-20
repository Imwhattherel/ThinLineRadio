// Copyright (C) 2025 Thinline Dynamic Solutions

package mapping

import "testing"

func TestSanitizeFalseMedicalAlarmToBurglary(t *testing.T) {
	tx := "ORDER RESPONSE FOR AN ALARM AT TMC THE ETM AT 21593 LORRAINE ROAD SHOWING TAMPER TAMPER"
	labels := []string{"ALARM DROP MEDICAL", "FIRE ALARM DROP", "BURGLARY"}
	got := SanitizeCallNatureAssignment(tx, "ALARM DROP MEDICAL", labels)
	if got != "BURGLARY" {
		t.Fatalf("got %q want BURGLARY", got)
	}
}

func TestSanitizeKeepsRealMedicalAlarm(t *testing.T) {
	tx := "MEDICAL ALARM AT 120 MAIN STREET PENDANT ACTIVATION"
	labels := []string{"ALARM DROP MEDICAL", "BURGLARY"}
	got := SanitizeCallNatureAssignment(tx, "ALARM DROP MEDICAL", labels)
	if got != "ALARM DROP MEDICAL" {
		t.Fatalf("got %q want ALARM DROP MEDICAL", got)
	}
}
