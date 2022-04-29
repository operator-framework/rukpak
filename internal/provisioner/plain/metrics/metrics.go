package metrics

import (
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

const (
	// metricsPath is the path that the provisioner exposes its metrics at.
	metricsPath = "/metrics"

	// metricsPort is the port that the provisioner exposes its metrics over http.
	metricsPort = 8383
)

const (
	nameLabel      = "Name"
	namespaceLabel = "Namespace"
)

var (
	bundleGaugeVec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bundle",
			Help: "Bundle Infomation. A positive value represents successful bundle unpack. A zero value represents bundle unpacking is pending. A negative value represents bundle unpacking has failed.",
		},
		[]string{nameLabel, namespaceLabel},
	)
)

// ServeMetrics enables the provisioner to serve prometheus metrics.
func ServeMetrics() error {
	// Register metrics for the operator with the prometheus.
	logrus.Info("[metrics] Registering provisioner metrics")

	err := registerMetrics()
	if err != nil {
		logrus.Infof("[metrics] Unable to register provisioner metrics: %v", err)
		return err
	}

	// Start the server and expose the registered metrics.
	logrus.Info("[metrics] Serving provisioner metrics")
	http.Handle(metricsPath, promhttp.Handler())

	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", metricsPort), nil)
		if err != nil {
			if err == http.ErrServerClosed {
				logrus.Errorf("Metrics (http) server closed")
				return
			}
			logrus.Errorf("Metrics (http) serving failed: %v", err)
		}
	}()

	return nil
}

// registerMetrics registers plain provisioner prometheus metrics.
func registerMetrics() error {
	// Register all of the metrics in the standard registry.
	prometheus.MustRegister(bundleGaugeVec)
	return nil
}

type BundleMetricsRecorder struct {
	bundle *rukpakv1alpha1.Bundle
	logger logr.Logger
}

func NewBundleMetricsRecorder(b *rukpakv1alpha1.Bundle, l logr.Logger) BundleMetricsRecorder {
	return BundleMetricsRecorder{b, l}
}

func (r *BundleMetricsRecorder) SetBundleMetric() {
	switch r.bundle.Status.Phase {
	case string(rukpakv1alpha1.PhaseUnpacked):
		r.logger.V(1).Info(fmt.Sprintf("Setting bundle{%s,%s} metric to 1", r.bundle.Name, r.bundle.Namespace))
		bundleGaugeVec.WithLabelValues(r.bundle.Name, r.bundle.Namespace).Set(1)
	case string(rukpakv1alpha1.PhaseFailing):
		r.logger.V(1).Info(fmt.Sprintf("Setting bundle{%s,%s} metric to -1", r.bundle.Name, r.bundle.Namespace))
		bundleGaugeVec.WithLabelValues(r.bundle.Name, r.bundle.Namespace).Set(-1)
	default:
		r.logger.V(1).Info(fmt.Sprintf("Setting bundle{%s,%s} metric to 0", r.bundle.Name, r.bundle.Namespace))
		bundleGaugeVec.WithLabelValues(r.bundle.Name, r.bundle.Namespace).Set(0)
	}
}

func (r *BundleMetricsRecorder) DeleteBundleMetric() {
	r.logger.V(1).Info(fmt.Sprintf("Deleting bundle{%s,%s} metric", r.bundle.Name, r.bundle.Namespace))
	bundleGaugeVec.DeleteLabelValues(r.bundle.Name, r.bundle.Namespace)
}
