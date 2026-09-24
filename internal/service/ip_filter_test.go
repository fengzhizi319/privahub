package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

func TestIPFilter_ExtendedRanges(t *testing.T) {
	f := NewIPFilter(nil)
	for _, ip := range []string{
		"169.254.169.254", "100.64.0.1", "0.0.0.0", "0.1.2.3", "127.0.0.2", "224.0.0.1", "255.255.255.255",
		"::1", "::", "fc00::1", "fd12::1", "fe80::1", "ff02::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"::ffff:169.254.169.254", "64:ff9b::a00:1",
	} {
		assert.True(t, f.IsIPInRange(net.ParseIP(ip)), ip)
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888", "::ffff:8.8.8.8"} {
		assert.False(t, f.IsIPInRange(net.ParseIP(ip)), ip)
	}
	// A custom list keeps the always-blocked ranges (metadata, unspecified, multicast).
	custom := NewIPFilter(&config.IPBlockConfig{List: []string{"8.8.8.0/24"}})
	assert.True(t, custom.IsIPInRange(net.ParseIP("169.254.169.254")))
	assert.True(t, custom.IsIPInRange(net.ParseIP("fe80::1")))
	assert.False(t, custom.IsIPInRange(net.ParseIP("1.1.1.1")))
}

func TestIPFilter_CheckURLAllResolvedAddresses(t *testing.T) {
	f := NewIPFilter(nil)
	f.resolve = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("10.0.0.1")}, nil
	}
	assert.Equal(t, errcode.FeatureTableIpFilter.Code, dataBizCode(f.CheckURL("http://multi.example")))
}

// DNS rebinding: the pre-flight resolution says "public", the connection goes
// to loopback; the connect-time check refuses it.
func TestIPFilter_ConnectTimeEnforcement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	f := NewIPFilter(nil)
	f.resolve = func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	ok, err := f.Detection(context.Background(), srv.URL)
	assert.False(t, ok)
	assert.Equal(t, errcode.FeatureTableIpFilter.Code, dataBizCode(err))

	_, err = f.HTTPClient(0).Get(srv.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrIPBlocked))

	_, err = f.DialContext(context.Background(), "tcp", strings.TrimPrefix(srv.URL, "http://"))
	assert.ErrorIs(t, err, ErrIPBlocked)

	// Disabled filter (dev) lets loopback through.
	ok, err = NewDisabledIPFilter().Detection(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIPFilter_NoRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redir.Close()
	resp, err := NewDisabledIPFilter().HTTPClient(0).Get(redir.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestMysqlChecker_GuardedDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	m := NewMysqlCheckerWithFilter(NewIPFilter(nil))
	err = m.TestConnection(context.Background(), &MysqlConfig{Endpoint: ln.Addr().String(), User: "u", Password: "p", Database: "d"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrIPBlocked)
}

func TestDatasource_ConnectivityErrorsAreGeneric(t *testing.T) {
	ds, _, _ := newTestDatasourceService(t)
	// A closed local port: the driver error ("connection refused") must not
	// reach the client.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()
	info, _ := json.Marshal(MysqlDatasourceInfo{Endpoint: addr, User: "u", Password: "p", Database: "d"})
	vo, err := ds.TestDatasource(context.Background(), &TestDatasourceRequest{Type: "MYSQL", DataSourceInfo: info})
	require.NoError(t, err)
	assert.False(t, vo.Success)
	assert.NotContains(t, strings.ToLower(vo.Message), "refused")
	assert.NotContains(t, vo.Message, addr)

	// With the filter enabled the loopback endpoint is refused up-front.
	ds.WithIPFilter(NewIPFilter(nil))
	vo, err = ds.TestDatasource(context.Background(), &TestDatasourceRequest{Type: "MYSQL", DataSourceInfo: info})
	require.NoError(t, err)
	assert.False(t, vo.Success)
}
