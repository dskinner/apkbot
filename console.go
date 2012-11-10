package main

import (
	"bytes"
	"dasa.cc/dae/context"
	"dasa.cc/dae/datastore"
	"dasa.cc/dae/handler"
	"dasa.cc/dae/render"
	"dasa.cc/dae/user"
	"fmt"
	"io"
	"labix.org/v2/mgo/bson"
	"net/http"
	"strings"
	"time"
)

//
func stacktraceCollapse(reports []*Report) (traces []string) {

	for _, report := range reports {

		var trace []string
		lines := strings.Split(report.StackTrace, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "...") || strings.HasPrefix(line, "at android") || strings.HasPrefix(line, "at com.android") || strings.HasPrefix(line, "at java") || strings.HasPrefix(line, "at dalvik") {
				continue
			} else {
				trace = append(trace, line)
			}
		}

		traces = append(traces, strings.Join(trace, "\n"))
	}

	return traces
}

// groupErrors iterates over apk.Errors and groups them by stack trace.
func groupErrors(reports []*Report) (groups []bson.M) {

	traces := stacktraceCollapse(reports)

	used := []string{}

	// iter backwards to show most recent traces first
	for i := len(reports) - 1; i >= 0; i-- {
		e := reports[i]
		trace := traces[i]

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
func attachGroupDetails(master *Report, reports []*Report) {

	var installIds []string

	mTrace := stacktraceCollapse([]*Report{master})[0]
	traces := stacktraceCollapse(reports)

	for i, report := range reports {
		rTrace := traces[i]

		if mTrace == rTrace {
			//if master.StackTrace == report.StackTrace {

			master.TotalErrors += 1

			if !containString(master.AndroidVersions, report.AndroidVersion) {
				master.AndroidVersions = append(master.AndroidVersions, report.AndroidVersion)
			}

			s := report.Brand + " - " + report.PhoneModel

			//if !containString(master.Brands, report.Brand) {
			//	master.Brands = append(master.Brands, report.Brand)
			//}

			if !containString(master.PhoneModels, s) {
				master.PhoneModels = append(master.PhoneModels, s)
			}
			if !containString(installIds, report.InstallationId) {
				installIds = append(installIds, report.InstallationId)
			}

			if report.Id != master.Id {
				master.ReportIds = append(master.ReportIds, report.Id)
			}
		}
	}

	master.UniqueInstalls = len(installIds)
}

//
func index(w http.ResponseWriter, r *http.Request) *handler.Error {
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

	db := datastore.New()
	defer db.Close()

	// get session user
	c := context.New(r)
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
			// get info from related reports to attach here
			var reports []*Report
			q = bson.M{"apkid": data.Active.Id}
			if err := db.C("reports").Find(q).All(&reports); err != nil {
				return handler.NewError(err, 500, "Error querying for apk reports.")
			}
			attachGroupDetails(data.Report, reports)
		} else {
			var reports []*Report
			q = bson.M{"apkid": data.Active.Id}
			if err := db.C("reports").Find(q).All(&reports); err != nil {
				return handler.NewError(err, 500, "Error querying for apk reports.")
			}
			data.Errors = groupErrors(reports)
		}
	}

	//
	render.Auto(w, r, data)

	return nil
}

// download provides the requested apk by the given bson.ObjectId
func download(w http.ResponseWriter, r *http.Request) *handler.Error {
	var (
		apk Apk
		buf bytes.Buffer
	)

	db := datastore.New()
	defer db.Close()

	q := bson.M{"_id": bson.ObjectIdHex(r.FormValue("id"))}
	err := db.C("apks").Find(q).One(&apk)
	if err != nil {
		return handler.NewError(err, 404, "No record of apk.")
	}

	file, err := db.FS().OpenId(apk.FileId)
	if err != nil {
		return handler.NewError(err, 404, "No such apk.")
	}

	io.Copy(&buf, file)

	filename := fmt.Sprintf("%s-%s.apk", apk.Badging.ApplicationLabel, apk.Time)

	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", "attachment;filename="+filename)
	w.Write(buf.Bytes())

	return nil
}

// icon retrieves a previously saved apk icon.
func icon(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	id := bson.ObjectIdHex(r.FormValue("id"))
	file, err := db.FS().OpenId(id)
	if err != nil {
		return handler.NewError(err, 404, "No such icon.")
	}

	var buf bytes.Buffer
	io.Copy(&buf, file)
	// TODO set content type correctly
	w.Header().Set("Content-type", "image/png")
	w.Write(buf.Bytes())

	return nil
}

func newuser(w http.ResponseWriter, r *http.Request) *handler.Error {
	render.Auto(w, r, nil)
	return nil
}

func init() {
	http.Handle("/console/", handler.New(handler.Auth, index))
	http.Handle("/console/download", handler.New(handler.Auth, download))
	http.Handle("/console/icon", handler.New(handler.Auth, icon))
	http.Handle("/console/newuser", handler.New(handler.Auth, newuser))
}
