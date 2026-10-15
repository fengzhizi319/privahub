package kuscia

import (
	"net/http"
	"net/url"
	"testing"
)

func transportOf(t *testing.T, c *Client) *http.Transport {
	t.Helper()
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport %T", c.httpClient.Transport)
	}
	return tr
}

// KusciaAPI lives on loopback / the private network: by default the client
// must not send requests through a developer's HTTP(S)_PROXY (a proxy answers
// "502" for unresolvable Kuscia hosts such as a missing tee lite).
func TestClient_IgnoresEnvironmentProxyByDefault(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:7897")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7897")
	t.Setenv("NO_PROXY", "")
	c, err := NewClientE(&ClientConfig{Host: "root-kuscia-lite-tee", Port: 8082, Protocol: "notls"})
	if err != nil {
		t.Fatal(err)
	}
	if transportOf(t, c).Proxy != nil {
		t.Fatal("default client must connect directly (Proxy == nil)")
	}
}

func TestClient_HTTPProxyOptions(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://kuscia.example:8082/api/v1/job/watch", nil)

	c, err := NewClientE(&ClientConfig{Host: "h", Port: 1, HTTPProxy: "http://proxy.internal:3128"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := transportOf(t, c).Proxy(req)
	if err != nil || u == nil || u.Host != "proxy.internal:3128" {
		t.Fatalf("explicit proxy: %v %v", u, err)
	}

	c, err = NewClientE(&ClientConfig{Host: "h", Port: 1, HTTPProxy: "env"})
	if err != nil || transportOf(t, c).Proxy == nil {
		t.Fatalf("env proxy should use http.ProxyFromEnvironment: %v", err)
	}

	for _, v := range []string{"", "none", "direct"} {
		c, err = NewClientE(&ClientConfig{Host: "h", Port: 1, HTTPProxy: v})
		if err != nil || transportOf(t, c).Proxy != nil {
			t.Fatalf("%q should mean direct: %v", v, err)
		}
	}

	c = NewClient(&ClientConfig{Host: "h", Port: 1, HTTPProxy: "::not a url"})
	if c.Err() == nil {
		t.Fatal("invalid http_proxy must be reported")
	}
}

// Pool endpoints without their own http_proxy inherit the default client's
// value, without persisting it into the endpoint.
func TestPool_EndpointInheritsHTTPProxy(t *testing.T) {
	def := NewClient(&ClientConfig{Host: "127.0.0.1", Port: 8082, HTTPProxy: "http://proxy.internal:3128"})
	p := NewClientPool(def, "")
	if err := p.Register(NodeEndpoint{DomainID: "tee", ClientConfig: ClientConfig{Host: "root-kuscia-lite-tee", Port: 8082}}); err != nil {
		t.Fatal(err)
	}
	c, ok := p.Lookup("tee")
	if !ok {
		t.Fatal("tee not registered")
	}
	u, _ := transportOf(t, c).Proxy(&http.Request{URL: &url.URL{Scheme: "http", Host: "root-kuscia-lite-tee:8082"}})
	if u == nil || u.Host != "proxy.internal:3128" {
		t.Fatalf("inherited proxy = %v", u)
	}
	for _, ep := range p.Endpoints() {
		if ep.DomainID == "tee" && ep.HTTPProxy != "" {
			t.Fatalf("inherited proxy must not be stored in the endpoint: %q", ep.HTTPProxy)
		}
	}
}
