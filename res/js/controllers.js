function RootCtrl($location) {
	$location.path("/login");
}


function UserMenuCtrl($rootScope, $scope, $http, $dialog, User) {

	var loggedIn = function() {
		$scope.page = "/partials/user-menu";
		$scope.user = User.get(function(data) {
			if (data.Name !== "") {
				$scope.userText = data.Name;
			} else {
				$scope.userText = data.Email;
			}
		});
	};

	var loggedOut = function() {
		$scope.page = "/partials/user-menu-login";
	};

	$http.get("/auth").
		success(loggedIn).
		error(loggedOut);
	
	$scope.$on("LoggedIn", loggedIn);
	$scope.$on("LoggedOut", loggedOut);

	$scope.logout = function() {
		$http.post("/logout").
			success(function() {
				$rootScope.$broadcast("LoggedOut");
				$location.path("/login");
			});
	};

	$scope.openUser = function() {
		var d = $dialog.dialog({
			backdrop: true,
			keyboard: true,
			backdropClick: true,
			dialogFade: true,
			backdropFade: true,
			templateUrl: "/partials/user",
			controller: UserCtrl
		});
		d.open().then(function(result) {
		});
	};
}


function LoginCtrl($rootScope, $scope, $http, $location) {

	$scope.login = function() {
		var vals = {
			"email": $scope.email,
			"password": $scope.password
		};

		$http.post("/login", vals).
			success(function(data) {
				$rootScope.$broadcast("LoggedIn");
				$location.path("/project");
			}).
			error(function(data) {
				$scope.err = data;
			});
	};

}


function UserCtrl($scope, dialog, User) {
	$scope.user = User.get();
	$scope.close = function() {
		dialog.close();
	};
}


function ProjectListCtrl($scope, $location, Project) {
	$scope.projects = Project.query();

	$scope.showProject = function(project) {
		$location.path("/project/" + project.Id + "/" + project.ApkIds[0]);
	};
}


function ProjectCtrl($rootScope, $scope, $location, Project, History) {

	// kinde of lame, use to keep logic out of template until ui-route is
	// is working and/or move to the new state router
	$scope.tpl = {
		errors: true,
		testers: false
	};

	$scope.showErrors = function() {
		$scope.tpl.errors = true;
		$scope.tpl.testers = false;
		$scope.page = "/partials/apk-errors";
	};

	$scope.showTesters = function() {
		$scope.tpl.errors = false;
		$scope.tpl.testers = true;
		$scope.page = "/partials/project-testers";
	};

	$scope.page = "/partials/apk-errors";

	$scope.getVersionString = function(apk) {
		return 'vc ' + apk.Badging.Package.VersionCode + ' vn ' + apk.Badging.Package.VersionName;
	};

	$scope.apkChange = function(apk) {
		var path = "/project/" + $scope.project.Id + "/" + apk.Id;
		if (path !== $location.url()) {
			$location.path(path);
		}
	};

	$scope.$on("$routeChangeSuccess", function(ev, route) {

		if ($scope.project !== undefined && $scope.project.Id === route.params.projectId) {
			return;
		}

		var loadHistory = function() {
			$scope.history = History.query({projectId: route.params.projectId}, selectApk);
		};

		var selectApk = function() {
			var r = $.grep($scope.history, function(e) { return e.Id == route.params.apkId; });
			if (r.length === 1) {
				$scope.apk = r[0];
			}
			$scope.apkChange($scope.apk);
		};

		$scope.project = Project.get({projectId: route.params.projectId}, loadHistory);

	});

}


function ApkTestersCtrl($scope) {

}


function ApkErrorListCtrl($scope, $location, $anchorScroll, $routeParams, Report) {
	$scope.opts = {
		page: {
			indices: [],
			index: 0,
			size: 100,
			num: 0
		}
	};

	$scope.showError = function(err) {
		// $location.hash(err.id);
		$location.path("/project/" + $scope.project.Id + "/" + $scope.apk.Id + "/error/" + err.id);
	};

	var showErrorsPage = function() {
		if ($scope.errorsAll === undefined) {
			return;
		}
		var start = ($scope.opts.page.index - 1) * $scope.opts.page.size;
		var end = start + $scope.opts.page.size;
		$scope.errors = $scope.errorsAll.slice(start, end);
		
		/*
		window.setTimeout(function() {
			$anchorScroll();
		}, 1000);
		*/
	};

	$scope.pageLeft = function() {
		$scope.opts.page.index = Math.max(1, $scope.opts.page.index - 1);
		showErrorsPage();
	};

	$scope.pageRight = function() {
		$scope.opts.page.index = Math.min($scope.opts.page.num, $scope.opts.page.index + 1);
		showErrorsPage();
	};

	$scope.pageOpen = function(i) {
		$scope.opts.page.index = i;
		showErrorsPage();
	};

	Report.query({apkId: $routeParams.apkId}, function(data) {
		$scope.errorsAll = data;
		$scope.opts.page.num = Math.ceil(data.length / $scope.opts.page.size);
		$scope.opts.page.indices = [];
		for (var i = 0; i < $scope.opts.page.num; i++) {
			$scope.opts.page.indices.push(i+1);
		}
		$scope.pageOpen(1);
	});
}


function ApkErrorCtrl($scope, $routeParams, $location, Report) {
	$scope.back = function() {
		$location.hash("");
		$location.path("/project/" + $routeParams.projectId + "/" + $routeParams.apkId);
	};

	$scope.anchorTo = function(a) {
		$location.hash(a);
	};

	$scope.apkError = Report.get({apkId: $routeParams.apkId, reportId: $routeParams.reportId});
}
