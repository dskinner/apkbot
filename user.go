package main

import (
	"dasa.cc/dae/context"
	"dasa.cc/dae/datastore"
	"dasa.cc/dae/handler"
	"dasa.cc/dae/render"
	"dasa.cc/dae/user"
	"labix.org/v2/mgo/bson"
	"net/http"
)

func userCurrent(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	c := context.New(r)
	u := user.Current(c, db)
	m := bson.M{"Name": u.Name, "Email": u.Email}

	render.Json(w, m)
	return nil
}

func userSetProfile(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	c := context.New(r)
	u := user.Current(c, db)

	u.Name = r.FormValue("name")[:100]
	u.Email = r.FormValue("email")[:100]

	if err := db.C("users").UpdateId(u.Id, u); err != nil {
		return handler.NewError(err, 500, "Error updating user profile")
	}

	return nil
}

func userSetPassword(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	c := context.New(r)
	u := user.Current(c, db)

	if !u.Validate(r.FormValue("oldpassword")) {
		return handler.NewError(nil, 400, "Old password incorrect!")
	}

	newPass := r.FormValue("password")
	if newPass != r.FormValue("repeatpassword") {
		return handler.NewError(nil, 400, "New password doesn't match!")
	}

	if len(newPass) > 100 {
		return handler.NewError(nil, 400, "Password length over 100. Seriously?!")
	}

	u.SetPassword(newPass)
	if err := db.C("users").UpdateId(u.Id, u); err != nil {
		return handler.NewError(err, 500, "Error updating user password!")
	}

	return nil
}

func init() {
	http.Handle("/user", handler.New(handler.Auth, userCurrent))
	http.Handle("/user/setprofile", handler.New(handler.Auth, userSetProfile))
	http.Handle("/user/setpassword", handler.New(handler.Auth, userSetPassword))
}
