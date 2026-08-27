package main

import (
	"flag"
	"os"
	"time"

	adaptivev1alpha1 "github.com/EdgeCDN-X/edgecdnx-plugin/api/v1alpha1"
	"github.com/EdgeCDN-X/edgecdnx-plugin/internal/quality"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	var prometheusURL string
	var interval time.Duration
	flag.StringVar(&prometheusURL, "prometheus-url", "http://monitoring-kube-prometheus-prometheus.monitoring.svc:9090", "Prometheus base URL")
	flag.DurationVar(&interval, "reconcile-interval", 5*time.Second, "NodeQuality refresh interval")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		os.Exit(1)
	}
	if err := adaptivev1alpha1.AddToScheme(scheme); err != nil {
		os.Exit(1)
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Cache:                  cache.Options{DefaultNamespaces: map[string]cache.Config{"edge-system": {}}},
		Metrics:                metricsserver.Options{BindAddress: ":8080"},
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		ctrl.Log.Error(err, "create manager")
		os.Exit(1)
	}
	reconciler := &quality.Reconciler{Client: mgr.GetClient(), Provider: quality.NewPrometheusProvider(prometheusURL, 3*time.Second), Interval: interval}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "setup controller")
		os.Exit(1)
	}
	_ = mgr.AddHealthzCheck("healthz", healthz.Ping)
	_ = mgr.AddReadyzCheck("readyz", healthz.Ping)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "run manager")
		os.Exit(1)
	}
}
