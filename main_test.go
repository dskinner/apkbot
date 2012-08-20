package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestLoginGet(t *testing.T) {

	s := httptest.NewServer(http.DefaultServeMux)
	defer s.Close()

	r, err := http.Get(s.URL + "/login")
	if err != nil {
		t.Error(err)
	}
	defer r.Body.Close()

	_, err = ioutil.ReadAll(r.Body)
	if err != nil {
		t.Error(err)
	}
	if r.StatusCode != http.StatusOK {
		t.Fatal("GET /login returned ", r.StatusCode)
	}
}

// _TestLoginPost is used by other tests.
func _TestLoginPost(t *testing.T, vals url.Values, ok bool) {

	s := httptest.NewServer(http.DefaultServeMux)
	defer s.Close()

	r, err := http.PostForm(s.URL+"/login", vals)
	if err != nil {
		t.Error(err)
	}
	defer r.Body.Close()

	_, err = ioutil.ReadAll(r.Body)
	if err != nil {
		t.Error(err)
	}

	// should always redirect with 302
	if r.StatusCode != http.StatusFound {
		t.Fatal("POST /login returned", r.StatusCode)
	}

	// check location header
	loc := r.Header["Location"]

	if len(loc) == 0 {
		t.Fatal("Header Location not set for login redirect")
	}

	var expected string
	if ok {
		expected = "/console/index"
	} else {
		expected = "/login"
	}

	if loc[0] != expected {
		t.Fatal("Redirect to wrong location", loc[0])
	}
}

func TestLoginPostGood(t *testing.T) {
	vals := url.Values{"email": {"daniel@dasa.cc"}, "password": {"testpass"}}
	_TestLoginPost(t, vals, true)
}

func TestLoginPostBadPassword(t *testing.T) {
	vals := url.Values{"email": {"daniel@dasa.cc"}, "password": {"jfieahld"}}
	_TestLoginPost(t, vals, false)
}
