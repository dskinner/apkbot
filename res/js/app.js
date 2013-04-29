angular.module("apkbot", ["services", "filters", "ui", "ui.bootstrap"]).
	config(function($httpProvider) {
		$httpProvider.defaults.transformRequest = function(data) {
			if (data !== undefined) {
				return $.param(data);
			}
		};
		$httpProvider.defaults.headers.post["Content-Type"] = "application/x-www-form-urlencoded; charset=UTF-8";
	}).
	config(function($routeProvider) {
		$routeProvider.
			when("/", {
				controller: RootCtrl
			}).
			when("/login", {
				controller: LoginCtrl,
				templateUrl: "/partials/login"
			}).
			when("/user", {
				controller: UserCtrl,
				templateUrl: "/partials/user"
			}).
			when("/project", {
				controller: ProjectListCtrl,
				templateUrl: "/partials/project-list"
			}).
			when("/project/:projectId/:apkId", {
				controller: ProjectCtrl,
				templateUrl: "/partials/project",
				reloadOnSearch: false
			}).
			when("/project/:projectId/:apkId/testers", {
				controller: ApkTestersCtrl,
				templateUrl: "/partials/project-testers"
			}).
			when("/project/:projectId/:apkId/error", {
				controller: ApkErrorListCtrl,
				templateUrl: "/partials/apk-errors"
			}).
			when("/project/:projectId/:apkId/error/:reportId", {
				controller: ApkErrorCtrl,
				templateUrl: "/partials/apk-error",
				reloadOnSearch: false
			}).
			otherwise({redirectTo: "/login"});
	});
