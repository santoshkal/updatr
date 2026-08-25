// Package main is the entry point for the updatr controller manager.
package main

import (
	"flag"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/santoshkal/updatr/internal/controller"
)

var (
	// scheme is the global runtime Scheme that holds type registrations.
	scheme = runtime.NewScheme()
	// setupLog is the logger used during manager setup (before mgr logger is ready).
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	// utilruntime.Must() panics if AddToScheme fails, surfacing scheme errors early.
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	// utilruntime.Must() registers corev1 types (Secret, ConfigMap, Pod).
	utilruntime.Must(corev1.AddToScheme(scheme))
	// utilruntime.Must() registers apps/v1 types (Deployment, StatefulSet).
	utilruntime.Must(appsv1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
	)

	// flag.StringVar() defines the metrics bind address flag.
	flag.StringVar(
		&metricsAddr,
		"metrics-bind-address",
		":8080",
		"The address the metric endpoint binds to.",
	)
	// flag.StringVar() defines the health probe bind address flag.
	flag.StringVar(
		&probeAddr,
		"health-probe-bind-address",
		":8081",
		"The address the probe endpoint binds to.",
	)
	// flag.BoolVar() defines the leader-election toggle flag.
	flag.BoolVar(
		&enableLeaderElection,
		"leader-elect",
		false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.",
	)

	// zap.Options{} configures the zap logger (development mode, etc.).
	opts := zap.Options{
		Development: true,
	}
	// opts.BindFlags() binds zap flags (--zap-* ) to the flag set.
	opts.BindFlags(flag.CommandLine)
	// flag.Parse() parses all defined flags from os.Args.
	flag.Parse()

	// zap.New() builds a logger from zap.Options.
	// zap.UseFlagOptions() returns functional options that apply flag values.
	// ctrl.SetLogger() installs it as the global controller-runtime logger.
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ctrl.GetConfigOrDie() loads kubeconfig (in-cluster or KUBECONFIG) or panics.
	kubeConfig := ctrl.GetConfigOrDie()

	// ctrl.NewManager() creates a manager that runs controllers and caches.
	// Caches are an interesting implementation in controller-runtime
	// that makes the reconcile loop efficient.

	// Refer to a detaild article on caches and reconcile at
	// https://kubernetes.io/blog/2026/07/29/controller-runtime-cache-explained/
	mgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "updatr.santoshkal.github.io",
	})
	if err != nil {
		// setupLog.Error() logs the manager creation failure at error level.
		setupLog.Error(err, "unable to start manager")
		// os.Exit() terminates the process with a non-zero exit code.
		os.Exit(1)
	}

	// &controller.SecretReconciler{} instantiates the Secret reconciler with mgr client/scheme.
	secretReconciler := &controller.SecretReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	// secretReconciler.SetupWithManager() registers the Secret controller with the manager.
	if err := secretReconciler.SetupWithManager(mgr); err != nil {
		// setupLog.Error() logs the failure to create the Secret controller.
		setupLog.Error(err, "unable to create controller", "controller", "Secret")
		// os.Exit() exits on setup failure.
		os.Exit(1)
	}

	// &controller.ConfigMapReconciler{} instantiates the ConfigMap reconciler.
	cmReconciler := &controller.ConfigMapReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	// cmReconciler.SetupWithManager() registers the ConfigMap controller.
	if err := cmReconciler.SetupWithManager(mgr); err != nil {
		// setupLog.Error() logs the failure to create the ConfigMap controller.
		setupLog.Error(err, "unable to create controller", "controller", "ConfigMap")
		// os.Exit() exits on setup failure.
		os.Exit(1)
	}

	// mgr.AddHealthzCheck() adds a health endpoint (healthz.Ping always returns nil).
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		// setupLog.Error() logs failure to add healthz check.
		setupLog.Error(err, "unable to set up health check")
		// os.Exit() exits if healthz cannot be configured.
		os.Exit(1)
	}
	// mgr.AddReadyzCheck() adds a readiness probe (healthz.Ping).
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		// setupLog.Error() logs failure to add readyz check.
		setupLog.Error(err, "unable to set up ready check")
		// os.Exit() exits if readyz cannot be configured.
		os.Exit(1)
	}

	// setupLog.Info() logs that the manager is starting.
	setupLog.Info("starting manager")
	// mgr.Start() starts the manager, blocking until context cancellation.
	// ctrl.SetupSignalHandler() returns a context that cancels on SIGINT/SIGTERM.
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		// setupLog.Error() logs the manager run failure.
		setupLog.Error(err, "problem running manager")
		// os.Exit() exits with failure status.
		os.Exit(1)
	}
}
