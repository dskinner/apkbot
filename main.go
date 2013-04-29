package main

import (
	"dasa.cc/dae"
	"dasa.cc/dae/datastore"
	"dasa.cc/dae/handler"
	"dasa.cc/dae/render"
	"dasa.cc/dae/user"
	"flag"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
)

var (
	adduser = flag.String("adduser", "", "add a new user with the given email")

	memprofile = flag.String("memprofile", "", "write memory profile to this file")

	debug  = flag.Bool("debug", false, "show stack on error")
	cache  = flag.Bool("cache", false, "primitive view cache")
	dbhost = flag.String("dbhost", "localhost", "")
	dbname = flag.String("dbname", "apkbot", "")

	router = mux.NewRouter()
)

func init() {
	dae.RegisterFileServer("res/")
	dae.ServeFile("/favicon.ico", "res/favicon.ico")

	router.Handle("/", handler.Auto)
	router.Handle("/auth", handler.Auth)
	router.Handle("/login", user.Login)
	router.Handle("/logout", user.Logout)

	router.Handle("/partials/{name}", handler.Auto)

	http.Handle("/", router)
}

func main() {
	flag.Parse()

	render.Cache = *cache
	handler.Debug = *debug

	datastore.DBHost = *dbhost
	datastore.DBName = *dbname

	if *adduser != "" {
		u := user.New()
		u.Email = *adduser
		u.SetPassword("qwerty")
		db := datastore.New()
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
