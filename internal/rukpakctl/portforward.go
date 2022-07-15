package rukpakctl

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServicePortForwarder struct {
	cfg       *rest.Config
	apiReader client.Reader

	ready     chan struct{}
	localPort uint16

	serviceName      string
	serviceNamespace string
	port             intstr.IntOrString
}

func NewServicePortForwarder(cfg *rest.Config, cl client.Reader, service types.NamespacedName, port intstr.IntOrString) *ServicePortForwarder {
	return &ServicePortForwarder{
		cfg:       cfg,
		apiReader: cl,
		ready:     make(chan struct{}),

		serviceName:      service.Name,
		serviceNamespace: service.Namespace,
		port:             port,
	}
}

func (pf *ServicePortForwarder) Start(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	endpoints := &corev1.Endpoints{}
	if err := pf.apiReader.Get(ctx, types.NamespacedName{Name: pf.serviceName, Namespace: pf.serviceNamespace}, endpoints); err != nil {
		return err
	}

	if len(endpoints.Subsets) == 0 || len(endpoints.Subsets[0].Addresses) == 0 {
		return fmt.Errorf("could not find available endpoint for %s service", pf.serviceName)
	}
	podName := endpoints.Subsets[0].Addresses[0].TargetRef.Name
	port := pf.port.IntVal
	if port == 0 {
		for _, p := range endpoints.Subsets[0].Ports {
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

func (pf *ServicePortForwarder) LocalPort(ctx context.Context) (uint16, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-pf.ready:
		return pf.localPort, nil
	}
}
