package main

import (
	"context"
	"flag"
	"os"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/dependency"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
	}))

	oamLog := ctrl.Log.WithName("oam-runtime")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
	if err != nil {
		oamLog.Error(err, "unable to create a controller manager")
		os.Exit(1)
	}

	l := logging.NewLogrLogger(oamLog)
	dependency.SetupGlobalDAGManager(l, mgr.GetClient())
	if err = oam.Setup(mgr, l); err != nil {
		oamLog.Error(err, "unable to setup oam core controller")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go dependency.GlobalManager.Start(ctx)

	oamLog.Info("starting the controller manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		oamLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
