package main

import "testing"

func TestDefaultDatabase(t *testing.T) {
	t.Setenv("DRAGONQR_DATABASE", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	if got := defaultDatabase(); got != "data/players.db" {
		t.Fatalf("local database = %q", got)
	}
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	if got := defaultDatabase(); got != clusterDatabase {
		t.Fatalf("cluster database = %q", got)
	}
	t.Setenv("DRAGONQR_DATABASE", "custom.db")
	if got := defaultDatabase(); got != "custom.db" {
		t.Fatalf("override database = %q", got)
	}
}
