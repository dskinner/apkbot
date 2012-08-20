package main

import (
	"bytes"
	"dasa.cc/dae"
	"dasa.cc/dae/user"
	"fmt"
	"io"
	"labix.org/v2/mgo/bson"
	"net/http"
	"strings"
	"time"
)

// groupErrors iterates over apk.Errors and groups them by stack trace.
func groupErrors(reports []*Report) (groups []bson.M) {

	traces := []string{}
	for _, e := range reports {
		traces = append(traces, e.StackTrace)
	}

	used := []string{}

	// iter backwards to show most recent traces first
	for i := len(reports) - 1; i >= 0; i-- {
		e := reports[i]
		trace := e.StackTrace

		if containString(used, trace) {
			continue
		}
		used = append(used, trace)

		count := countString(traces, trace)

		s := strings.Split(trace, "\n")
		groups = append(groups, bson.M{"stacktrace": s[0], "count": count, "id": e.Id, "duration": time.Now().Sub(e.Time)})
	}

	return groups
}

//
func index(w http.ResponseWriter, r *http.Request) *dae.Error {
	// response struct
	var data struct {
		User    *user.User
		Apks    []*Apk
		History []*Apk
		Active  *Apk
		Errors  []bson.M
		Report  *Report
	}

	var q bson.M

	db := dae.NewDB()
	defer db.Close()

	// get session user
	c := dae.NewContext(r)
	data.User = user.Current(c, db)

	// get user projects
	projects := ProjectsByUser(data.User, db)

	// get newest apk entry for all projects
	data.Apks = ProjectsOverview(db, projects)

	// get history of specified package name if requested, default to first if not
	var project *Project
	name := r.FormValue("name")
	if name != "" {
		// match package name
		for _, p := range projects {
			if p.PackageName == name {
				project = p
				break
			}
		}
	} else if len(projects) > 0 {
		// default to first if available
		project = projects[0]
	}

	if project != nil {
		data.History = project.History(db)
	}

	// set active apk to newest in history, else if id was given, locate apk in history and set as active
	activeId := r.FormValue("active")
	if activeId == "" && data.History != nil {
		data.Active = data.History[0]
	} else {
		for _, apk := range data.History {
			if activeId == apk.Id.Hex() {
				data.Active = apk
				break
			}
		}
	}

	// setup reports
	if data.Active != nil {
		reportId := r.FormValue("report")
		if reportId != "" {
			db.C("reports").FindId(bson.ObjectIdHex(reportId)).One(&data.Report)
		} else {
			var reports []*Report
			q = bson.M{"apkid": data.Active.Id}
			if err := db.C("reports").Find(q).All(&reports); err != nil {
				return dae.NewError(err, 500, "Error querying for apk reports.")
			}
			data.Errors = groupErrors(reports)
		}
	}

	//
	dae.Render(w, r, data)

	return nil
}

// download provides the requested apk by the given bson.ObjectId
func download(w http.ResponseWriter, r *http.Request) *dae.Error {
	var (
		apk Apk
		buf bytes.Buffer
	)

	db := dae.NewDB()
	defer db.Close()

	q := bson.M{"_id": bson.ObjectIdHex(r.FormValue("id"))}
	err := db.C("apks").Find(q).One(&apk)
	if err != nil {
		return dae.NewError(err, 404, "No record of apk.")
	}

	file, err := db.FS().OpenId(apk.FileId)
	if err != nil {
		return dae.NewError(err, 404, "No such apk.")
	}

	io.Copy(&buf, file)

	filename := fmt.Sprintf("%s-%s.apk", apk.Badging.ApplicationLabel, apk.Time)

	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", "attachment;filename="+filename)
	w.Write(buf.Bytes())

	return nil
}

// icon retrieves a previously saved apk icon.
func icon(w http.ResponseWriter, r *http.Request) *dae.Error {
	db := dae.NewDB()
	defer db.Close()

	id := bson.ObjectIdHex(r.FormValue("id"))
	file, err := db.FS().OpenId(id)
	if err != nil {
		return dae.NewError(err, 404, "No such icon.")
	}

	var buf bytes.Buffer
	io.Copy(&buf, file)
	// TODO set content type correctly
	w.Header().Set("Content-type", "image/png")
	w.Write(buf.Bytes())

	return nil
}

func init() {
	http.Handle("/console/index", dae.Handler(dae.Auth).Add(index))
	http.Handle("/console/download", dae.Handler(dae.Auth).Add(download))
	http.Handle("/console/icon", dae.Handler(dae.Auth).Add(icon))
}
