/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testhost

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/dynamicrp/server"
	"github.com/radius-project/radius/pkg/ucp/config"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/hosting"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	queueprovider "github.com/radius-project/radius/pkg/ucp/queue/provider"
	secretprovider "github.com/radius-project/radius/pkg/ucp/secret/provider"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Start(t *testing.T) (*TestHost, *testserver.TestServer) {
	config := &dynamicrp.Config{
		Environment: hostoptions.EnvironmentOptions{
			Name:         "test",
			RoleLocation: v1.LocationGlobal,
		},
		Queue: queueprovider.QueueProviderOptions{
			Provider: queueprovider.TypeInmemory,
			Name:     "dynamic-rp",
		},
		Secrets: secretprovider.SecretProviderOptions{
			Provider: secretprovider.TypeInMemorySecret,
		},
		Server: hostoptions.ServerOptions{
			// Initialized dynamically when the server is started.
		},
		Storage: dataprovider.StorageProviderOptions{
			Provider: dataprovider.TypeInMemory,
		},
		UCP: config.UCPOptions{
			Kind: config.UCPConnectionKindDirect,
			Direct: &config.UCPDirectConnectionOptions{
				Endpoint: "http://localhost:8080",
			},
		},
	}

	options, err := dynamicrp.NewOptions(context.Background(), config)
	require.NoError(t, err)

	return StartWithOptions(t, options)
}

func StartWithOptions(t *testing.T, options *dynamicrp.Options) (*TestHost, *testserver.TestServer) {
	// Allocate a random free port.
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to allocate port")

	options.Config.Server.Host = "localhost"
	options.Config.Server.Port = listener.Addr().(*net.TCPAddr).Port
	err = listener.Close()
	require.NoError(t, err, "failed to close listener")

	options.Config.Server.PathBase = "/" + uuid.New().String()
	baseURL := fmt.Sprintf(
		"http://%s:%d%s",
		options.Config.Server.Host,
		options.Config.Server.Port)

	host, err := server.NewServer(options)
	require.NoError(t, err, "failed to create server")

	ctx, cancel := context.WithCancel(testcontext.New(t))
	errs, messages := host.RunAsync(ctx)

	go func() {
		for msg := range messages {
			t.Logf("Message: %s", msg)
		}
	}()

	th := &TestHost{
		baseURL:     baseURL,
		host:        host,
		messages:    messages,
		cancel:      cancel,
		stoppedChan: errs,
		t:           t,
	}
	t.Cleanup(th.Close)

	return th, startUCP(t, baseURL)
}

func startUCP(t *testing.T, url string) *testserver.TestServer {
	return testserver.StartWithETCD(t, func(options modules.Options) []modules.Initializer {
		options.DynamicRP.URL = url
		return api.DefaultModules(options)
	})
}

// TestHost is a test server for the dynamic RP. Do not construct this type directly, use one of the
// Start functions.
type TestHost struct {
	// baseURL is the base URL of the server, including the path base.
	baseURL string

	// host is the hosting process running the component.
	host *hosting.Host

	// messages is the channel that will receive lifecycle messages from the host.
	messages <-chan hosting.LifecycleMessage

	// cancel is the function to call to stop the server.
	cancel context.CancelFunc

	// stoppedChan is the channel that will be closed when the server has stopped.
	stoppedChan <-chan error

	// shutdown is used to ensure that Close is only called once.
	shutdown sync.Once

	// t is the testing.T instance to use for assertions.
	t *testing.T
}

// Close shuts down the server and will block until shutdown completes.
func (th *TestHost) Close() {
	// We're being picking about resource cleanup here, because unless we are picky we hit scalability
	// problems in tests pretty quickly.
	th.shutdown.Do(func() {
		// Shut down the host.
		th.cancel()

		if th.stoppedChan != nil {
			<-th.stoppedChan // host stopped
		}
	})
}

// BaseURL returns the base URL of the server, including the path base.
//
// This should be used as a URL prefix for all requests to the server.
func (th *TestHost) BaseURL() string {
	return th.baseURL
}

// Client returns the HTTP client to use to make requests to the server.
func (th *TestHost) Client() *http.Client {
	return http.DefaultClient
}
