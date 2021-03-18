package main

import (
	"flag"
	"io"
	"os"
	"strconv"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller"
	appController "github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2"
	webhook "github.com/crossplane/oam-kubernetes-runtime/pkg/webhook/v1alpha2"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	_ = crdv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr, logFilePath, leaderElectionNamespace string
	var enableLeaderElection, logCompress bool
	var logRetainDate int
	var certDir string
	var webhookPort int
	var useWebhook bool
	var debugLogs bool
	var controllerArgs controller.Args

	flag.BoolVar(&useWebhook, "use-webhook", false, "Enable Admission Webhook")
	flag.StringVar(&certDir, "webhook-cert-dir", "/k8s-webhook-server/serving-certs", "Admission webhook cert/key dir.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "admission webhook listen address")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "",
		"Determines the namespace in which the leader election configmap will be created.")
	flag.StringVar(&logFilePath, "log-file-path", "", "The file to write logs to")
	flag.IntVar(&logRetainDate, "log-retain-date", 7, "The number of days of logs history to retain.")
	flag.BoolVar(&logCompress, "log-compress", true, "Enable compression on the rotated logs.")
	flag.BoolVar(&debugLogs, "debug-logs", false, "Enable the debug logs useful for development")
	flag.IntVar(&controllerArgs.RevisionLimit, "revision-limit", 50,
		"RevisionLimit is the maximum number of revisions that will be maintained. The default value is 50.")
	flag.BoolVar(&controllerArgs.ApplyOnceOnly, "apply-once-only", false,
		"For the purpose of some production environment that workload or trait should not be affected if no spec change")
	flag.Parse()

	// setup logging
	var w io.Writer
	if len(logFilePath) > 0 {
		w = zapcore.AddSync(&lumberjack.Logger{
			Filename: logFilePath,
			MaxAge:   logRetainDate, // days
			Compress: logCompress,
		})
	} else {
		w = os.Stdout
	}

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = debugLogs
		o.DestWritter = w
	}))

	oamLog := ctrl.Log.WithName("oam-kubernetes-runtime")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "oam-kubernetes-runtime",
		LeaderElectionNamespace: leaderElectionNamespace,
		Port:                    webhookPort,
		CertDir:                 certDir,
	})
	if err != nil {
		oamLog.Error(err, "unable to create a controller manager")
		os.Exit(1)
	}

	if useWebhook {
		oamLog.Info("OAM webhook enabled, will serving at :" + strconv.Itoa(webhookPort))
		if err = webhook.Add(mgr); err != nil {
			oamLog.Error(err, "unable to setup the webhook for core controller")
			os.Exit(1)
		}

	}

	if err = appController.Setup(mgr, controllerArgs, logging.NewLogrLogger(oamLog)); err != nil {
		oamLog.Error(err, "unable to setup the oam core controller")
		os.Exit(1)
	}
	oamLog.Info("starting the controller manager")
	if controllerArgs.ApplyOnceOnly {
		oamLog.Info("applyOnceOnly is enabled that means workload or trait only apply once if no spec change even they are changed by others")
	}
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		oamLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
