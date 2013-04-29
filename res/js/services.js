angular.module("services", ["ngResource"]).
	factory("Project", function($resource) {
		var Project = $resource("/project/:projectId");
		return Project;
	}).
	factory("History", function($resource) {
		var History = $resource("/project/:projectId/history");
		return History;
	}).
	factory("Apk", function($resource) {
		var Apk = $resource("/apk/:apkId");
		return Apk;
	}).
	factory("Report", function($resource) {
		var Report = $resource("/report/:apkId/:reportId");
		return Report;
	}).
	factory("User", function($resource) {
		var User = $resource("/user");
		return User;
	});
