package main

import (
	"dasa.cc/dae"
	"dasa.cc/dae/user"
	"flag"
	"labix.org/v2/mgo/bson"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
)

func login(w http.ResponseWriter, r *http.Request) *dae.Error {
	c := dae.NewContext(r)

	if r.Method == "GET" {
		flashes := c.Session().Flashes()
		c.Session().Save(r, w)
		dae.Render(w, r, flashes)
		return nil
	}

	db := dae.NewDB()
	defer db.Close()

	u, err := user.FindEmail(db, r.FormValue("email"))
	if err != nil {
		c.Session().AddFlash("User not found!")
		c.Session().Save(r, w)
		http.Redirect(w, r, "/login", 302)
		return nil
	}

	if u.Validate(r.FormValue("password")) {
		user.SetCurrent(c, u)
		c.Session().Save(r, w)
		http.Redirect(w, r, "/console/index", 302)
	} else {
		c.Session().AddFlash("Password doesn't match!")
		c.Session().Save(r, w)
		http.Redirect(w, r, "/login", 302)
	}

	return nil
}

func logout(w http.ResponseWriter, r *http.Request) *dae.Error {
	c := dae.NewContext(r)
	user.DelCurrent(c)
	c.Session().Save(r, w)
	http.Redirect(w, r, "/login", 302)
	return nil
}

func profile(w http.ResponseWriter, r *http.Request) *dae.Error {
	db := dae.NewDB()
	defer db.Close()

	c := dae.NewContext(r)
	u := user.Current(c, db)

	errs := []string{}
	msgs := []string{}

	for _, flash := range c.Session().Flashes() {
		f := flash.(string)
		if f[len(f)-1] == '!' {
			errs = append(errs, f)
		} else {
			msgs = append(msgs, f)
		}
	}
	c.Session().Save(r, w)

	dae.Render(w, r, bson.M{"User": u, "Errors": errs, "Messages": msgs})

	return nil
}

func profileUpdate(w http.ResponseWriter, r *http.Request) *dae.Error {
	db := dae.NewDB()
	defer db.Close()

	c := dae.NewContext(r)
	u := user.Current(c, db)

	// TODO truncate?
	u.Name = r.FormValue("name")

	if email := r.FormValue("email"); email != "" {
		u.Email = email
	}

	if err := db.C("users").UpdateId(u.Id, u); err != nil {
		return dae.NewError(err, 500, "Error updating user profile")
	}

	c.Session().AddFlash("Changes saved.")
	c.Session().Save(r, w)

	http.Redirect(w, r, "/console/profile", http.StatusFound)
	return nil
}

func profilePassword(w http.ResponseWriter, r *http.Request) *dae.Error {

	// redirect on return
	defer http.Redirect(w, r, "/console/profile", http.StatusFound)

	// setup env
	db := dae.NewDB()
	defer db.Close()

	c := dae.NewContext(r)
	defer c.Session().Save(r, w)

	u := user.Current(c, db)

	if !u.Validate(r.FormValue("oldpassword")) {
		c.Session().AddFlash("Old password incorrect!")
		return nil
	}

	newPass := r.FormValue("password")
	if newPass != r.FormValue("repeatpassword") {
		c.Session().AddFlash("New password doesn't match!")
		return nil
	}

	u.SetPassword(newPass)
	if err := db.C("users").UpdateId(u.Id, u); err != nil {
		return dae.NewError(err, 500, "Error updating user password!")
	}

	c.Session().AddFlash("Changes saved.")
	return nil
}

type StringSlice []string

func (slice StringSlice) Contains(s string) bool {
	for _, val := range slice {
		if val == s {
			return true
		}
	}
	return false
}

func (slice StringSlice) Count(s string) (count int) {
	for _, val := range slice {
		if val == s {
			count++
		}
	}
	return count
}

func test() {
	var slice []string
	slice = append(slice, "one")
	slice = append(slice, "two")

	StringSlice(slice).Contains("one")
	StringSlice(slice).Count("two")
}

// containString is a helper for determining if []string contains string.
func containString(slc []string, s string) bool {
	for _, v := range slc {
		if v == s {
			return true
		}
	}
	return false
}

// countString is a helper for counting the number of instances of string
// in []string.
func countString(slc []string, s string) (count int) {
	for _, v := range slc {
		if v == s {
			count++
		}
	}
	return count
}

func init() {
	dae.Cache = false
	dae.Debug = false

	res := http.FileServer(http.Dir("res/"))
	http.Handle("/img/", res)
	http.Handle("/css/", res)
	http.Handle("/js/", res)

	http.Handle("/login", dae.Handler(login))
	http.Handle("/logout", dae.Handler(logout))
	http.Handle("/console/profile", dae.NewHandler(dae.Auth, profile))
	http.Handle("/console/profile/update", dae.NewHandler(dae.Auth, profileUpdate))
	http.Handle("/console/profile/password", dae.NewHandler(dae.Auth, profilePassword))
}

var adduser = flag.String("adduser", "", "add a new user with the given email")
var memprofile = flag.String("memprofile", "", "write memory profile to this file")

func main() {
	flag.Parse()

	if *adduser != "" {
		u := user.New()
		u.Email = *adduser
		u.SetPassword("qwerty")
		db := dae.NewDB()
		defer db.Close()
		if err := db.C("users").Insert(u); err != nil {
			log.Fatal(err)
		}
		log.Print("user added, password is `qwerty`.")
	} else {
		if *memprofile != "" {

			c := make(chan os.Signal)
			signal.Notify(c)
			go func() {
				for sig := range c {
					log.Printf("Received %v", sig)
					f, err := os.Create(*memprofile)
					if err != nil {
						log.Fatal(err)
					}
					pprof.WriteHeapProfile(f)
					f.Close()
				}
			}()
		}
		log.Fatal(http.ListenAndServe("localhost:8090", nil))

	}
}
