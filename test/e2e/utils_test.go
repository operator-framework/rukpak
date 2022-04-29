package e2e

import (
	"bytes"
	"context"
	"strconv"
	"strings"

	. "github.com/onsi/gomega"
	promClient "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/kubernetes"
)

func conditionsSemanticallyEqual(a, b metav1.Condition) bool {
	return a.Type == b.Type && a.Status == b.Status && a.Reason == b.Reason && a.Message == b.Message
}

func conditionsLooselyEqual(a, b metav1.Condition) bool {
	return a.Type == b.Type && a.Status == b.Status && a.Reason == b.Reason && strings.Contains(b.Message, a.Message)
}

func extractMetricPortFromPod(pod *corev1.Pod) string {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == "metrics" {
				return strconv.Itoa(int(port.ContainerPort))
			}
		}
	}
	return "-1"
}

func getMetricsFromPod(client *kubernetes.Clientset, pod *corev1.Pod) []Metric {
	mfs := make(map[string]*promClient.MetricFamily)
	EventuallyWithOffset(1, func() error {
		raw, err := client.CoreV1().RESTClient().Get().
			Namespace(pod.GetNamespace()).
			Resource("pods").
			SubResource("proxy").
			Name(net.JoinSchemeNamePort("", pod.GetName(), extractMetricPortFromPod(pod))).
			Suffix("metrics").
			Do(context.Background()).Raw()
		if err != nil {
			return err
		}
		var p expfmt.TextParser
		mfs, err = p.TextToMetricFamilies(bytes.NewReader(raw))
		if err != nil {
			return err
		}
		return nil
	}).Should(Succeed())

	var metrics []Metric
	for family, mf := range mfs {
		if strings.HasPrefix(family, "go_") || strings.HasPrefix(family, "process_") || strings.HasPrefix(family, "promhttp_") {
			continue
		}
		for _, metric := range mf.Metric {
			m := Metric{
				Family: family,
			}
			if len(metric.GetLabel()) > 0 {
				m.Labels = make(map[string][]string)
			}
			for _, pair := range metric.GetLabel() {
				m.Labels[pair.GetName()] = append(m.Labels[pair.GetName()], pair.GetValue())
			}
			if u := metric.GetUntyped(); u != nil {
				m.Value = u.GetValue()
			}
			if g := metric.GetGauge(); g != nil {
				m.Value = g.GetValue()
			}
			if c := metric.GetCounter(); c != nil {
				m.Value = c.GetValue()
			}
			metrics = append(metrics, m)
		}
	}
	return metrics
}

func getPodWithLabel(client *kubernetes.Clientset, label string) *corev1.Pod {
	listOptions := metav1.ListOptions{LabelSelector: label}
	var podList *corev1.PodList
	EventuallyWithOffset(1, func() (numPods int, err error) {
		podList, err = client.CoreV1().Pods(defaultSystemNamespace).List(context.Background(), listOptions)
		if podList != nil {
			numPods = len(podList.Items)
		}

		return
	}).Should(Equal(1), "number of pods never scaled to one")

	return &podList.Items[0]
}
