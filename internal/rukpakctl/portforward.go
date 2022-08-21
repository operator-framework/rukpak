package rukpakctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// ServicePortForwarder forwards a port from a local port to a Kubernetes service.
type ServicePortForwarder struct {
	cfg *rest.Config
	cl  kubernetes.Interface

	ready     chan struct{}
	localPort uint16

	serviceName      string
	serviceNamespace string
	port             intstr.IntOrString
}

// NewServicePortForwarder creates a new ServicePortForwarder.
func NewServicePortForwarder(cfg *rest.Config, service types.NamespacedName, port intstr.IntOrString) (*ServicePortForwarder, error) {
	cl, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &ServicePortForwarder{
		cfg:   cfg,
		cl:    cl,
		ready: make(chan struct{}),

		serviceName:      service.Name,
		serviceNamespace: service.Namespace,
		port:             port,
	}, nil
}

// Start starts the port-forward defined by the ServicePortForwarder and blocks until the provided context is closed.
// When the provided context is closed, the forwarded port is also closed. This function opens a random local port,
// which can be discovered using the LocalPort method.
func (pf *ServicePortForwarder) Start(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	var subset corev1.EndpointSubset
	if err := wait.PollImmediateUntil(time.Second*1, func() (bool, error) {
		endpoints, err := pf.cl.CoreV1().Endpoints(pf.serviceNamespace).Get(ctx, pf.serviceName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if len(endpoints.Subsets) == 0 || len(endpoints.Subsets[0].Addresses) == 0 {
			return false, nil
		}
		subset = endpoints.Subsets[0]
		return true, nil
	}, ctx.Done()); err != nil {
		if errors.Is(err, ctx.Err()) {
			return fmt.Errorf("could not find available endpoint for %s service", pf.serviceName)
		}
		return err
	}

	podName := subset.Addresses[0].TargetRef.Name
	port := pf.port.IntVal
	if port == 0 {
		for _, p := range subset.Ports {
			if p.Name == pf.port.StrVal {
				port = p.Port
				break
			}
		}
	}
	if port == 0 {
		return fmt.Errorf("could not find port %q for service %q", pf.port.String(), pf.serviceName)
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", pf.serviceNamespace, podName)
	host := strings.TrimLeft(pf.cfg.Host, "htps:/")
	serverURL := url.URL{Scheme: "https", Path: path, Host: host}

	roundTripper, upgrader, err := spdy.RoundTripperFor(pf.cfg)
	if err != nil {
		return err
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)
	stopChan, readyChan := make(chan struct{}, 1), make(chan struct{}, 1)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)

	forwarder, err := portforward.New(dialer, []string{fmt.Sprintf("0:%d", port)}, stopChan, readyChan, out, errOut)
	if err != nil {
		return err
	}

	eg.Go(func() error {
		if err = forwarder.ForwardPorts(); err != nil { // Locks until stopChan is closed.
			return err
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		close(stopChan)
		return nil
	})

	// readyChan will be closed when the forwarded ports are ready.
	<-readyChan
	if errOut.String() != "" {
		return fmt.Errorf(errOut.String())
	}

	forwardedPorts, err := forwarder.GetPorts()
	if err != nil {
		return err
	}
	pf.localPort = forwardedPorts[0].Local
	close(pf.ready)
	return eg.Wait()
}

// LocalPort returns the local port on which the port forward is listening. It automatically
// waits until the port forward is configured, so there is no need for callers to coordinate
// calls between Start and LocalPort (other than that Start must be called at some point for
// a local port to eventually become ready). If the provided context is closed prior to the
// local port becoming ready, LocalPort returns the context's error.
func (pf *ServicePortForwarder) LocalPort(ctx context.Context) (uint16, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-pf.ready:
		return pf.localPort, nil
	}
}
