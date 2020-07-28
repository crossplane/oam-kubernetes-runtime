package main

import (
	"flag"
	"io"
	"os"
	"strconv"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
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

type rootArgs struct {
	// the address the metric endpoint binds to.
	metricsAddr string
	// enable leader election for controller manager.
	// enabling this will ensure there is only one active controller manager
	enableLeaderElection bool
	// number of days log files to retain
	logRetainDate int
	// path of log file output
	logFilePath string
	// compresse the rotated logs
	logCompress bool
	// admission webhook cert/key dir
	certDir string
	// admission webhook listen address
	webhookPort int
	// enable admission webhook
	useWebhook bool
}

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	args := rootArgs{}
	controllerArgs := controller.Args{}

	flag.StringVar(&args.metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&args.enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&controllerArgs.DefaultRevisionLimit, "revision-limit", 10,
		"RevisionLimit is the maximum number of revisions that will be maintained. The default value is 10.")
	flag.StringVar(&args.logFilePath, "log-file-path", "", "The path of log file output.")
	flag.IntVar(&args.logRetainDate, "log-retain-date", 7, "The number of days of logs history to retain.")
	flag.BoolVar(&args.logCompress, "log-compress", true, "Enable compression on the rotated logs.")
	flag.BoolVar(&args.useWebhook, "use-webhook", false, "Enable Admission Webhook")
	flag.StringVar(&args.certDir, "webhook-cert-dir", "/k8s-webhook-server/serving-certs", "Admission webhook cert/key dir.")
	flag.IntVar(&args.webhookPort, "webhook-port", 9443, "admission webhook listen address")
	flag.Parse()

	// setup logging
	var w io.Writer
	if len(args.logFilePath) > 0 {
		w = zapcore.AddSync(&lumberjack.Logger{
			Filename: args.logFilePath,
			MaxAge:   args.logRetainDate, // days
			Compress: args.logCompress,
		})
	} else {
		w = os.Stdout
	}

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
		o.DestWritter = w
	}))

	oamLog := ctrl.Log.WithName("oam-kubernetes-runtime")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: args.metricsAddr,
		LeaderElection:     args.enableLeaderElection,
		LeaderElectionID:   "oam-kubernetes-runtime",
		Port:               args.webhookPort,
		CertDir:            args.certDir,
	})
	if err != nil {
		oamLog.Error(err, "unable to create a controller manager")
		os.Exit(1)
	}

	if args.useWebhook {
		oamLog.Info("OAM webhook enabled, will serving at :" + strconv.Itoa(args.webhookPort))
		webhook.Add(mgr)
	}

	if err = appController.Setup(mgr, controllerArgs, logging.NewLogrLogger(oamLog)); err != nil {
		oamLog.Error(err, "unable to setup the oam core controller")
		os.Exit(1)
	}
	oamLog.Info("starting the controller manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		oamLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
