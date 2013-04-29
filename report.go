package main

import (
	"dasa.cc/dae/datastore"
	"dasa.cc/dae/handler"
	"dasa.cc/dae/render"
	"encoding/csv"
	"fmt"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo/bson"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type Report struct {
	Id    bson.ObjectId `_id`
	ApkId bson.ObjectId
	Time  time.Time

	// Group details TODO get this out of here
	Brands          []string
	PhoneModels     []string
	ReportIds       []bson.ObjectId
	AndroidVersions []string
	UniqueInstalls  int
	TotalErrors     int

	// ACRA details
	AndroidVersion       string `ANDROID_VERSION`
	AppVersionCode       string `APP_VERSION_CODE`
	AvailableMemSize     string `AVAILABLE_MEM_SIZE`
	Brand                string `BRAND`
	Build                string `BUILD`
	CrashConfiguration   string `CRASH_CONFIGURATION`
	CustomData           string `CUSTOM_DATA`
	DeviceFeatures       string `DEVICE_FEATURES`
	DeviceId             string `DEVICE_ID`
	Display              string `DISPLAY`
	Dropbox              string `DROPBOX`
	DumpSysMemInfo       string `DUMPSYS_MEMINFO`
	Environment          string `ENVIRONMENT`
	EventsLog            string `EVENTSLOG`
	FilePath             string `FILE_PATH`
	InitialConfiguration string `INITIAL_CONFIGURATION`
	InstallationId       string `INSTALLATION_ID`
	IsSilent             string `IS_SILENT`
	Logcat               string `LOGCAT`
	PackageName          string `PACKAGE_NAME`
	PhoneModel           string `PHONE_MODEL`
	Product              string `PRODUCT`
	RadioLog             string `RADIOLOG`
	ReportId             string `REPORT_ID`
	SettingsSecure       string `SETTINGS_SECURE`
	SettingsSystem       string `SETTINGS_SYSTEM`
	SharedPreferences    string `SHARED_PREFERENCES`
	StackTrace           string `STACK_TRACE`
	TotalMemSize         string `TOTAL_MEM_SIZE`
	UserAppStartDate     string `USER_APP_START_DATE`
	UserComment          string `USER_COMMENT`
	UserCrashDate        string `USER_CRASH_DATE`
	UserEmail            string `USER_EMAIL`
}

// report is a web handler that saves ACRA reports.
func report(w http.ResponseWriter, r *http.Request) *handler.Error {

	if err := r.ParseForm(); err != nil {
		return handler.NewError(err, 500, "Error parsing form data.")
	}

	db := datastore.New()
	defer db.Close()

	// find apk
	var apk *Apk
	name := r.FormValue("PACKAGE_NAME")
	vc := r.FormValue("APP_VERSION_CODE")
	// TODO use user submitted formid that identifies user with public hash for locating correct package
	q := bson.M{"badging.package.name": name, "badging.package.versionCode": vc}
	if err := db.C("apks").Find(q).Sort("-time").One(&apk); err != nil {
		// return 200 so acra doesn't keep resubmitting the report
		return handler.NewError(err, 200, fmt.Sprintf("No apk by given name and version code found: NAME %s VC %s", name, vc))
	}

	// save report using generic map to keep values not accounted for in Report struct.
	m := bson.M{}
	for k, v := range r.Form {
		if len(v) == 1 {
			m[k] = v[0]
		}
	}
	reportId := bson.NewObjectId()
	m["_id"] = reportId
	m["apkid"] = apk.Id
	m["time"] = time.Now()

	if err := db.C("reports").Insert(m); err != nil {
		return handler.NewError(err, 500, "Error inserting new report")
	}

	apk.ReportIds = append([]bson.ObjectId{reportId}, apk.ReportIds...)
	if err := db.C("apks").UpdateId(apk.Id, apk); err != nil {
		return handler.NewError(err, 500, "Error updating apk reportids")
	}

	// TODO send email notify if desired by user

	return nil
}

// ignoreField is a static list of fields to not export when creating a CSV.
func ignoreField(name string) bool {
	switch name {
	case "Id", "ApkId", "Time", "Brands", "PhoneModels", "ReportIds", "AndroidVersions", "UniqueInstalls", "TotalErrors":
		return true
	}
	return false
}

// listFields returns CSV header values as a slice.
func listFields(data interface{}) (fields []string) {
	t := reflect.ValueOf(data).Type()
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Name
		if ignoreField(name) {
			continue
		}
		fields = append(fields, t.Field(i).Name)
	}
	return fields
}

func exportCSV(w http.ResponseWriter, r *http.Request) *handler.Error {

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment;filename=export.csv")

	wrtr := csv.NewWriter(w)

	var sampleReport Report
	csvHeaders := listFields(sampleReport)
	wrtr.Write(csvHeaders)

	db := datastore.New()
	defer db.Close()

	var reports []*Report
	q := bson.M{"apkid": bson.ObjectIdHex(r.FormValue("id"))}
	if err := db.C("reports").Find(q).All(&reports); err != nil {
		return handler.NewError(err, 500, "Error querying for apk reports.")
	}

	for _, report := range reports {
		var record []string
		s := reflect.ValueOf(report).Elem()
		t := s.Type()
		for i := 0; i < s.NumField(); i++ {
			f := s.Field(i)
			if ignoreField(t.Field(i).Name) {
				continue
			}
			record = append(record, f.String())
		}

		wrtr.Write(record)
	}

	return nil
}

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

// containString is a helper for determining if []string contains string.
func containString(slc []string, s string) bool {
	for _, v := range slc {
		if v == s {
			return true
		}
	}
	return false
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

		var count int
		for _, v := range traces {
			if v == trace {
				count++
			}
		}

		s := strings.Split(trace, "\n")
		groups = append(groups, bson.M{"stacktrace": s[0], "count": count, "id": e.Id, "duration": e.Time})
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

func errorDetails(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	// get session user
	// c := context.New(r)
	// u := user.Current(c, db)

	apkId := mux.Vars(r)["apkId"]
	reportId := mux.Vars(r)["reportId"]

	var report *Report
	db.C("reports").FindId(bson.ObjectIdHex(reportId)).One(&report)
	// get info from related reports to attach here
	var reports []*Report
	q := bson.M{"apkid": bson.ObjectIdHex(apkId)} // TODO confirm user has access to project
	if err := db.C("reports").Find(q).All(&reports); err != nil {
		return handler.NewError(err, 500, "Error querying for apk reports.")
	}
	attachGroupDetails(report, reports)

	render.Json(w, report)
	return nil
}

func errorsByApkId(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	apkId := bson.ObjectIdHex(mux.Vars(r)["apkId"])

	var reports []*Report
	q := bson.M{"apkid": apkId}
	if err := db.C("reports").Find(q).All(&reports); err != nil {
		return handler.NewError(err, 500, "Error querying for apk reports.")
	}

	render.Json(w, groupErrors(reports))
	return nil
}

func init() {
	router.Handle("/report", handler.New(report))
	router.Handle("/report_error", handler.New(report)) // compat for apps in the wild

	router.Handle("/report/{apkId}", handler.New(handler.Auth, errorsByApkId))
	router.Handle("/report/{apkId}/exportCSV", handler.New(handler.Auth, exportCSV))
	router.Handle("/report/{apkId}/{reportId}", handler.New(handler.Auth, errorDetails))
}
