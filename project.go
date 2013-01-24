package main

import (
	"dasa.cc/dae/datastore"
	"dasa.cc/dae/user"
	"labix.org/v2/mgo/bson"
)

type Project struct {
	Id          bson.ObjectId `_id`
	PackageName string
	UserIds     []bson.ObjectId
	ApkIds      []bson.ObjectId
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
	return projects
}
