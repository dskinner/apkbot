package main

import (
	"dasa.cc/dae"
	"dasa.cc/dae/user"
	"testing"
)

func TestProjects(t *testing.T) {
	db := dae.NewDB()
	defer db.Close()

	u, err := user.FindEmail(db, "daniel@dasa.cc")
	if err != nil {
		t.Fatal(err)
	}

	if projects := ProjectsByUser(u, db); projects == nil {
		t.Fatal("Projects returned nil")
	}
}
