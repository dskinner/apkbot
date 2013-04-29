package main

import (
	"archive/zip"
	"bytes"
	"dasa.cc/dae/context"
	"dasa.cc/dae/datastore"
	"dasa.cc/dae/handler"
	"dasa.cc/dae/render"
	"dasa.cc/dae/user"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"labix.org/v2/mgo/bson"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
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

// upload is a web handler for receiving an apk.
func upload(w http.ResponseWriter, r *http.Request) *handler.Error {

	// dont use memory for holding apk
	if err := r.ParseMultipartForm(0); err != nil {
		return handler.NewError(err, 500, "Unable to parse multipart form.")
	}

	f, _, err := r.FormFile("apk")
	if err != nil {
		return handler.NewError(err, 500, "Form file \"apk\" does not exist")
	}

	// dump badging and locate appropriate icon name within apk
	m := dumpBadging(f.(*os.File).Name())
	k := fmt.Sprintf("application-icon-%s", m["densities"].([]interface{})[0])
	res := m[k].(string)

	// locate tmp file and extract icon
	fi, err := f.(*os.File).Stat()
	if err != nil {
		return handler.NewError(err, 500, "Can't stat file.")
	}

	zr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return handler.NewError(err, 500, "Not a valid zip archive")
	}

	icon := zipReadBytes(zr, res)

	// Link current user to package name, save badging, apk file, and icon file.
	db := datastore.New()
	defer db.Close()

	u := user.Current(context.New(r), db)

	apkFile, err := db.FS().Create("")
	if err != nil {
		return handler.NewError(err, 500, "Can't save apk")
	}

	defer apkFile.Close()
	io.Copy(apkFile, f)

	iconFile, err := db.FS().Create("")
	if err != nil {
		return handler.NewError(err, 500, "Can't save icon")
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
		return handler.NewError(err, 500, "Error inserting apk info.")
	}

	project := ProjectByName(u, apkName)
	project.ApkIds = append([]bson.ObjectId{apkId}, project.ApkIds...)
	if _, err = db.C("projects").UpsertId(project.Id, project); err != nil {
		return handler.NewError(err, 500, "Error upserting project.")
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

	id := mux.Vars(r)["iconId"]

	if id == "" {
		w.Header().Set("Content-type", "image/png")
		w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 31, 21, 196, 137, 0, 0, 0, 13, 73, 68, 65, 84, 8, 215, 99, 96, 96, 96, 96, 0, 0, 0, 5, 0, 1, 94, 243, 42, 58, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130})
		return nil
	}

	file, err := db.FS().OpenId(bson.ObjectIdHex(id))
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

func apkById(w http.ResponseWriter, r *http.Request) *handler.Error {
	db := datastore.New()
	defer db.Close()

	apkId := bson.ObjectIdHex(mux.Vars(r)["id"])

	var apk *Apk
	// TODO confirm user has access to apk
	if err := db.C("apks").FindId(apkId).One(&apk); err != nil {
		return handler.NewError(err, 500, "Failed to locate apk.")
	}

	render.Json(w, apk)
	return nil
}

// register our web handlers.
func init() {
	router.Handle("/apk/upload", handler.New(handler.Auth, upload))
	router.Handle("/apk/{apkId}", handler.New(handler.Auth, apkById))
	router.Handle("/apk/{apkId}/download", handler.New(handler.Auth, download))

	router.Handle("/apk/icon/", handler.New(handler.Auth, icon))
	router.Handle("/apk/icon/{iconId}", handler.New(handler.Auth, icon))
}
