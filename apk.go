package main

import (
	"archive/zip"
	"bytes"
	"dasa.cc/dae"
	"dasa.cc/dae/user"
	"fmt"
	"io"
	"labix.org/v2/mgo/bson"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"encoding/csv"
	"reflect"
)

type Apk struct {
	Id            bson.ObjectId `_id`
	FileId        bson.ObjectId
	IconId        bson.ObjectId
	ProguardMapId *bson.ObjectId
	Time          time.Time
	ReportIds     []bson.ObjectId

	Badging struct {
		ApplicationLabel string `application-label`

		Package struct {
			Name        string `name`
			VersionCode string `versionCode`
			VersionName string `versionName`
		}
	}
}

type Report struct {
	Id    bson.ObjectId `_id`
	ApkId bson.ObjectId
	Time  time.Time

	// Group details
	Brands          []string
	PhoneModels          []string
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

func ignoreField(name string) bool {
	switch name {
	case "Id", "ApkId", "Time", "Brands", "PhoneModels", "ReportIds", "AndroidVersions", "UniqueInstalls", "TotalErrors":
		return true
	}
	return false
}

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

func exportCSV(w http.ResponseWriter, r *http.Request) *dae.Error {

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment;filename=export.csv")

	wrtr := csv.NewWriter(w)

	var sampleReport Report
	csvHeaders := listFields(sampleReport)
	wrtr.Write(csvHeaders)

	db := dae.NewDB()
	defer db.Close()

	var reports []*Report
	q := bson.M{"apkid": bson.ObjectIdHex(r.FormValue("id"))}
	if err := db.C("reports").Find(q).All(&reports); err != nil {
		return dae.NewError(err, 500, "Error querying for apk reports.")
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

// report is a web handler that saves ACRA reports.
func report(w http.ResponseWriter, r *http.Request) *dae.Error {

	if err := r.ParseForm(); err != nil {
		return dae.NewError(err, 500, "Error parsing form data.")
	}

	db := dae.NewDB()
	defer db.Close()

	// find apk
	var apk *Apk
	name := r.FormValue("PACKAGE_NAME")
	vc := r.FormValue("APP_VERSION_CODE")
	// TODO use user submitted formid that identifies user with public hash for locating correct package
	q := bson.M{"badging.package.name": name, "badging.package.versionCode": vc}
	if err := db.C("apks").Find(q).Sort("-time").One(&apk); err != nil {
		// return 200 so acra doesn't keep resubmitting the report
		return dae.NewError(err, 200, fmt.Sprintf("No apk by given name and version code found: NAME %s VC %s", name, vc))
	}

	// save report
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
		return dae.NewError(err, 500, "Error inserting new report")
	}

	apk.ReportIds = append([]bson.ObjectId{reportId}, apk.ReportIds...)
	if err := db.C("apks").UpdateId(apk.Id, apk); err != nil {
		return dae.NewError(err, 500, "Error updating apk reportids")
	}

	// send email notify if desired by user
	//user.

	return nil
}

// upload is a web handler for receiving an apk.
func upload(w http.ResponseWriter, r *http.Request) *dae.Error {

	// dont use memory for holding apk
	if err := r.ParseMultipartForm(0); err != nil {
		return dae.NewError(err, 500, "Unable to parse multipart form.")
	}

	f, _, err := r.FormFile("apk")
	if err != nil {
		return dae.NewError(err, 500, "Form file \"apk\" does not exist")
	}

	// dump badging and locate appropriate icon name within apk
	m := dumpBadging(f.(*os.File).Name())
	k := fmt.Sprintf("application-icon-%s", m["densities"].([]interface{})[0])
	res := m[k].(string)

	// locate tmp file and extract icon
	fi, err := f.(*os.File).Stat()
	if err != nil {
		return dae.NewError(err, 500, "Can't stat file.")
	}

	zr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return dae.NewError(err, 500, "Not a valid zip archive")
	}

	icon := zipReadBytes(zr, res)

	// Link current user to package name, save badging, apk file, and icon file.
	db := dae.NewDB()
	defer db.Close()

	u := user.Current(dae.NewContext(r), db)

	apkFile, err := db.FS().Create("")
	if err != nil {
		return dae.NewError(err, 500, "Can't save apk")
	}

	defer apkFile.Close()
	io.Copy(apkFile, f)

	iconFile, err := db.FS().Create("")
	if err != nil {
		return dae.NewError(err, 500, "Can't save icon")
	}

	defer iconFile.Close()
	iconFile.Write(icon)

	// using bson.M to store all badging values, including those not specified in struct
	apkId := bson.NewObjectId()
	apkName := m["package"].(map[string]interface{})["name"].(string)
	apkM := bson.M{
		"_id":     apkId,
		"fileid":  apkFile.Id(),
		"iconid":  iconFile.Id(),
		"time":    time.Now(),
		"badging": m,
	}

	if err = db.C("apks").Insert(apkM); err != nil {
		return dae.NewError(err, 500, "Error inserting apk info.")
	}

	project := ProjectByName(u, apkName)
	project.ApkIds = append([]bson.ObjectId{apkId}, project.ApkIds...)
	if _, err = db.C("projects").UpsertId(project.Id, project); err != nil {
		return dae.NewError(err, 500, "Error upserting project.")
	}

	http.Redirect(w, r, "/console/index", 302)
	return nil
}

// zipReadBytes opens an apk, locates the specified resource,
// and returns []bytes for resource.
func zipReadBytes(zr *zip.Reader, res string) []byte {
	buf := new(bytes.Buffer)
	for _, zf := range zr.File {
		if zf.Name == res {
			rc, err := zf.Open()
			if err != nil {
				panic(err)
			}
			io.Copy(buf, rc)
			rc.Close()
			return buf.Bytes()
		}
	}

	panic("File not found.")
}

// dumpBadging calls aapt from the OS and parses the output into a mapping.
// Later on, we map key/value pairs from the mapping to a struct so that apks
// are backwards compatible for making use of new information not currently in
// use by the site.
func dumpBadging(path string) map[string]interface{} {
	out, err := exec.Command("aapt", "d", "badging", path).Output()
	if err != nil {
		panic(err)
	}

	badging := make(map[string]interface{})

	var key string
	var value interface{}

	for _, line := range strings.Split(fmt.Sprintf("%s", out), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}

		tmp := strings.SplitN(line, ":", 2)
		key = tmp[0]
		value = tmp[1]

		switch key {
		case "package", "application", "launchable-activity":
			value = parseMap(value.(string))
		case "supports-screens", "densities":
			value = parseSlice(value.(string))
		default:
			value = strings.Replace(value.(string), "'", "", -1)
		}

		switch badging[key].(type) {
		case interface{}:
			badging[key] = []interface{}{badging[key], value}
		case []interface{}:
			badging[key] = append(badging[key].([]interface{}), value)
		case nil:
			badging[key] = value
		}
	}

	return badging
}

// parseSlice returns an array of interface{} given string.
// TODO this might be made simpler with scanf
func parseSlice(s string) []interface{} {
	s = strings.Replace(s, "'", "", -1)

	a := []interface{}{}

	for _, v := range strings.Split(s, " ") {
		if v == "" {
			continue
		}
		a = append(a, v)
	}

	return a
}

// parseMap returns a mapping of key/value pairs based on input string
// TODO this might be made simpler with fmt.Scanf
func parseMap(s string) map[string]interface{} {

	var key string
	escape := false
	start := 0

	m := make(map[string]interface{})

	for i, c := range s {
		switch c {
		case ' ':
			if !escape {
				start = i + 1
			}
		case '=':
			if !escape {
				key = s[start:i]
				start = i + 1
			}
		case '\'':
			if escape && start != i {
				m[key] = s[start:i]
			} else {
				start++
			}
			escape = !escape
		}
	}

	return m
}

// register our web handlers.
func init() {
	http.Handle("/console/upload", dae.NewHandler(dae.Auth, upload))
	http.Handle("/console/exportCSV", dae.NewHandler(dae.Auth, exportCSV))
	http.Handle("/report", dae.Handler(report))
	http.Handle("/report_error", dae.Handler(report)) // compat for apps in the wild
}
