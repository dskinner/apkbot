package main

import (
	"dasa.cc/dae/context"
	"dasa.cc/dae/datastore"
	"dasa.cc/dae/handler"
	"dasa.cc/dae/render"
	"dasa.cc/dae/user"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo/bson"
	"net/http"
)

type Project struct {
	Id          bson.ObjectId `_id`
	PackageName string
	UserIds     []bson.ObjectId
	ApkIds      []bson.ObjectId

	LastApk *Apk
}

func (p *Project) LoadLastApk(db *datastore.DB) {
	if len(p.ApkIds) == 0 {
		return
	}
	if err := db.C("apks").FindId(p.ApkIds[0]).One(&p.LastApk); err != nil {
		panic(err)
	}
}

// ProjectByName finds a project for the given user by the given name
// or creates a new one if one does not exist.
func ProjectByName(u *user.User, name string) *Project {
	var projects []Project

	db := datastore.New()
	defer db.Close()

	q := bson.M{"userids": u.Id}
	if err := db.C("projects").Find(q).All(&projects); err != nil {
		panic(err)
	}

	for _, project := range projects {
		if project.PackageName == name {
			return &project
		}
	}

	// new project if one does not exist
	project := &Project{Id: bson.NewObjectId(), PackageName: name}
	project.UserIds = append(project.UserIds, u.Id)
	return project
}

func ProjectById(u *user.User, id bson.ObjectId) (project *Project) {
	db := datastore.New()
	defer db.Close()

	q := bson.M{"_id": id, "userids": u.Id}
	if err := db.C("projects").Find(q).One(&project); err != nil {
		panic(err)
	}
	project.LoadLastApk(db)
	return
}

// ProjectHistory returns a slice of all apks for project.
func (p *Project) History(db *datastore.DB) (apks []*Apk) {
	q := bson.M{"_id": bson.M{"$in": p.ApkIds}}
	db.C("apks").Find(q).Sort("-time").All(&apks)
	return apks
}

// ProjectsOverview provides a list of last apk uploaded for each project in slice.
func ProjectsOverview(db *datastore.DB, projects []*Project) (apks []*Apk) {
	var ids []bson.ObjectId
	for _, p := range projects {
		ids = append(ids, p.ApkIds[0])
	}
	q := bson.M{"_id": bson.M{"$in": ids}}
	if err := db.C("apks").Find(q).All(&apks); err != nil {
		panic(err)
	}

	return apks
}

// Projects retrieves a list of the user's projects from db.
func ProjectsByUser(u *user.User, db *datastore.DB) (projects []*Project) {
	q := bson.M{"userids": u.Id}
	if err := db.C("projects").Find(q).All(&projects); err != nil {
		panic(err)
	}
	for _, project := range projects {
		project.LoadLastApk(db)
	}
	return projects
}

func projectsByUser(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	c := context.New(r)
	u := user.Current(c, db)

	projects := ProjectsByUser(u, db)
	render.Json(w, projects)

	return nil
}

func projectById(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	c := context.New(r)
	u := user.Current(c, db)

	id := bson.ObjectIdHex(mux.Vars(r)["projectId"])

	project := ProjectById(u, id)
	render.Json(w, project)

	return nil
}

func projectHistory(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	c := context.New(r)
	u := user.Current(c, db)

	id := bson.ObjectIdHex(mux.Vars(r)["projectId"])

	project := ProjectById(u, id)
	render.Json(w, project.History(db))

	return nil
}

func init() {
	router.Handle("/project", handler.New(handler.Auth, projectsByUser))
	router.Handle("/project/{projectId}", handler.New(handler.Auth, projectById))
	router.Handle("/project/{projectId}/history", handler.New(handler.Auth, projectHistory))
}
